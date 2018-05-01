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
	Height    int
	Overwrite bool
	Target    string
	VArgs     string
	AArgs     string
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
		AArgs:  "-c:a aac -b:a 96k",
		VArgs:  "-c:v libx264 -crf 25 -r 25 -preset faster -profile:v main -level 3.1 -tune film",
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

func fileExist(filename string) bool {
	if _, err := os.Stat("/path/to/whatever"); err == nil {
		return true
	}
	return false
}

func splitPath(filename string) (string, string, string) {
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

	dir, basename, _ := splitPath(filename)

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

func getAudioOpts(audio AudioInfo) []string {
	if audio.AAC && audio.BitRate <= 120 {
		return []string{"-c:a", "copy"}
	} else {
		return strings.Split(config.AArgs, " ")
	}
}

func canCopyVideo(video VideoInfo) bool {
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

func getVideoOpts(video VideoInfo) []string {
	if canCopyVideo(video) {
		return []string{"-c:v", "copy"}
	}

	args := strings.Split(config.VArgs, " ")
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
		args = append(args, "-vf ", strings.Join(vf, ","))
	}
	return args
}

func removeEmptyArgs(args []string) []string {
	result := make([]string, 0, len(args))
	for _, v := range args {
		if v != "" {
			result = append(result, v)
		}
	}
	return result
}

func main() {
	log.Printf("config:%q", config)
	if config.Target != "" {
		if _, err := os.Lstat(config.Target); os.IsNotExist(err) {
			os.MkdirAll(config.Target, 0755)
		}
	}
	filename := os.Args[1]
	mediaInfo := getMediaInfo(filename)
	if len(mediaInfo.Sub) > 0 {
		sub := mediaInfo.Sub[0]
		target := filename + "." + sub.Sub
		if config.Target != "" {
			_, basename, ext := splitPath(filename)
			target = filepath.Join(config.Target, basename+ext+"."+sub.Sub)
		}
		args := []string{
			"-i",
			filename,
			"-an",
			"-vn",
			"-c:s:" + strconv.Itoa(sub.Index),
			sub.Sub,
			"-n",
			target,
		}
		_, err := exec.Command("ffmpeg", args...).Output()
		if err != nil {
			log.Fatalln("extract sub failed:%s,%s", strings.Join(args, " "), err.Error())
		}
	}
	dir, basename, _ := splitPath(filename)
	args := []string{
		"-i", filename, "-sn", "-metadata", `title="` + basename + `"`,
	}
	if config.Overwrite {
		args = append([]string{"-y"}, args...)
	}
	args = append(args, getVideoOpts(mediaInfo.Video[0])...)
	args = append(args, getAudioOpts(mediaInfo.Audio[0])...)

	target := filepath.Join(dir, basename+".mp4cvt.mp4")
	if config.Target != "" {
		target = filepath.Join(config.Target, basename+".mp4")
	}
	args = append(args, target)
	log.Printf("ffmpeg %s", strings.Join(args, " "))
	cmd := exec.Command("ffmpeg", removeEmptyArgs(args)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
}

func getMediaInfo(filename string) MediaInfo {
	args := "-v quiet -print_format json -show_format -show_streams"
	ffprobeArgs := strings.Split(args, " ")
	stdout, err := exec.Command("ffprobe", append(ffprobeArgs, filename)...).Output()
	if err != nil {
		log.Fatalf("exec ffprobe failed:%s,%s", args, err.Error())
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
