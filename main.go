package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v2"
)

var config = loadConfig()

// Config config
type Config struct {
	Height int
	CRF    int
	Tune   string
	Preset string
	FPS    int
}

// VideoInfo video
type VideoInfo struct {
	Filename string
	AVC      bool
	Profile  string
	Level    int
	Width    int
	Height   int
	BitRate  int
}

// AudioInfo audio
type AudioInfo struct {
	AAC     bool
	BitRate int
}

// SubInfo audio
type SubInfo struct {
	Index int
	Sub   string
}

// MediaInfo media
type MediaInfo struct {
	Video []VideoInfo
	Audio []AudioInfo
	Sub   []SubInfo
}

// NewConfig create new config with default value
func NewConfig() Config {
	return Config{
		Height: 720,
		CRF:    25,
		Tune:   "film",
		Preset: "veryfast",
		FPS:    25,
	}
}

func loadConfig() Config {
	config := NewConfig()
	confstr, err := ioutil.ReadFile("mp4cvt.yaml")
	if err != nil {
		log.Println("load mp4cvt.yaml failed:", err.Error())
	} else {
		err = yaml.Unmarshal(confstr, &config)
		if err != nil {
			log.Println("load mp4cvt.yaml failed:", err.Error())
		}
	}

	return config
}

func splitArgs(str string, args ...interface{}) []string {
	items := strings.Split(str, " ")
	pos := 0
	for i := range items {
		if items[i] == "%" {
			items[i] = fmt.Sprintf("%v", args[pos])
			pos++
		}
	}
	return items
}

func run(cmdname string, args ...string) error {
	cmd := exec.Command(cmdname, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func getAudioOpts(audio AudioInfo) []string {
	if audio.AAC && audio.BitRate <= 120 {
		return []string{"-c:a", "copy"}
	} else {
		return []string{"-c:a", "libfaac", "-b:a", "96k"}
	}
}

func fileExist(filename string) bool {
	if _, err := os.Stat("/path/to/whatever"); err == nil {
		return true
	}
	return false
}

func splitFilename(filename string) (string, string, string) {
	dir, base := filepath.Split(filename)
	ext := filepath.Ext(filename)
	basename := base[0 : len(base)-len(ext)]
	return dir, basename, ext
}
func getSubfile(filename string) string {
	subfile := filename + ".srt"
	if fileExist(subfile) {
		return subfile
	}
	subfile = filename + ".ass"
	if fileExist(subfile) {
		return subfile
	}

	dir, basename, _ := splitFilename(filename)

	subfile = filepath.Join(dir, basename+".ass")
	if fileExist(subfile) {
		return subfile
	}
	subfile = filepath.Join(dir, basename+".srt")
	if fileExist(subfile) {
		return subfile
	}

	return ""
}

func getVideoOpts(video VideoInfo) []string {
	canCopy := func() bool {
		if !video.AVC {
			return false
		}
		if video.Height > config.Height {
			return false
		}
		if getSubfile(video.Filename) != "" {
			return false
		}
		if video.BitRate > 600000 {
			return false
		}
		return true
	}
	if canCopy() {
		return []string{"-c:v", "copy"}
	}

	args := fmt.Sprintf("-c:v libx264 -crf %d -r %d -preset %s -profile:v main -level 3.1 -tune %s", config.CRF, config.FPS, config.Preset, config.Tune)
	vf := make([]string, 0, 10)
	if video.Height > config.Height {
		sar := float64(video.Width) / float64(video.Height)
		height := config.Height
		width := int(math.Floor((float64(height)*sar)/16) * 16)
		vf = append(vf, fmt.Sprintf("scale=%d:%d", width, height))
	}
	subfile := getSubfile(video.Filename)
	if subfile != "" {
		ext := filepath.Ext(subfile)
		if ext == ".ass" {
			vf = append(vf, fmt.Sprintf(`ass="%s"`, subfile))
		} else if ext == ".srt" {
			vf = append(vf, fmt.Sprintf(`subtitles="%s"`, subfile))
		}
	}
	if len(vf) > 0 {
		args = args + " -vf " + strings.Join(vf, ",")
	}
	return strings.Split(args, " ")
}

func main() {
	_ = config
	filename := os.Args[1]
	mediaInfo := getMediaInfo(filename)
	if len(mediaInfo.Sub) > 0 {
		sub := mediaInfo.Sub[0]
		args := splitArgs("-i % -an -vn -c:s:% % -n %.%", filename, sub.Index, sub.Sub, filename, sub.Sub)
		_, err := exec.Command("ffmpeg", args...).Output()
		if err != nil {
			log.Fatalln("extract sub failed:", err.Error())
		}
	}
	dir, basename, ext := splitFilename(filename)
	args := []string{
		"-i", filename, "-sn", "-metadata", `title="` + basename + `"`,
	}
	args = append(args, getVideoOpts(mediaInfo.Video[0])...)
	args = append(args, getAudioOpts(mediaInfo.Audio[0])...)
	args = append(args, filepath.Join(dir, basename+".mp4cvt"+ext))
	log.Println(args)
	run("ffmpeg", args...)
}

func getMediaInfo(filename string) MediaInfo {
	ffprobeArgs := strings.Split("-v quiet -print_format json -show_format -show_streams", " ")
	stdout, err := exec.Command("ffprobe", append(ffprobeArgs, filename)...).Output()
	if err != nil {
		log.Fatalln("exec ffprobe failed:", err.Error())
	}

	var data struct {
		Streams []struct {
			CodecType string `json:"codec_type"`
			CodecName string `json:"codec_name"`
			Profile   string `json:"profile"`
			Level     int    `json:"level"`
			Width     int    `json:"width"`
			Height    int    `json:"height"`
			BitRate   string `json:"bit_rate"`
			Index     int    `json:"index"`
		} `json:"streams"`
	}
	json.Unmarshal(stdout, &data)
	mediaInfo := MediaInfo{
		make([]VideoInfo, 0),
		make([]AudioInfo, 0),
		make([]SubInfo, 0),
	}

	for _, stream := range data.Streams {
		switch stream.CodecType {
		case "video":
			videoInfo := VideoInfo{
				Filename: filename,
				Width:    stream.Width,
				Height:   stream.Height,
			}
			videoInfo.BitRate, err = strconv.Atoi(stream.BitRate)
			if err != nil {
				videoInfo.BitRate = 9999999
			}
			if stream.CodecName == "h264" {
				videoInfo.AVC = true
				videoInfo.Profile = strings.ToLower(stream.Profile)
				videoInfo.Level = stream.Level
			}
			mediaInfo.Video = append(mediaInfo.Video, videoInfo)
		case "audio":
			if stream.CodecName == "aac" {
				audioInfo := AudioInfo{
					AAC: true,
				}
				audioInfo.BitRate, err = strconv.Atoi(stream.BitRate)
				if err != nil {
					audioInfo.BitRate = 9999999
				} else {
					audioInfo.BitRate = audioInfo.BitRate / 1000
				}
				mediaInfo.Audio = append(mediaInfo.Audio, audioInfo)
			} else {
				mediaInfo.Audio = append(mediaInfo.Audio, AudioInfo{})
			}
		case "subtitle":
			mediaInfo.Sub = append(mediaInfo.Sub, SubInfo{
				stream.Index, stream.CodecName,
			})
		}
	}
	return mediaInfo
}
