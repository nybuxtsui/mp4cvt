#!/usr/bin/env node
'use strict';

//let exec = require('child_process').exec;
let path = require('path');
let fs = require('fs');
let util = require('util');

let config = {
    height: 480,
    crf: 26,
    tune: 'animation',
    tmpdir: '/Volumes/ramdisk',
    _$_: ''
}

function get_subfile(filename) {
    let subfile = filename + '.srt';
    if (fs.existsSync(subfile)) {
        return subfile;
    }
    subfile = filename + '.ass';
    if (fs.existsSync(subfile)) {
        return subfile;
    }

    let ext = path.extname(filename);
    let dir = path.dirname(filename);
    let base = path.basename(filename, ext);

    subfile = path.join(dir, base + '.ass');
    if (fs.existsSync(subfile)) {
        return subfile;
    }
    subfile = path.join(dir, base + '.srt');
    if (fs.existsSync(subfile)) {
        return subfile;
    }

    return null;
}

function can_copy_video(info) {
    if (!info.isavc) {
        return false;
    }
    if (info.profile != 'high' && info.profile != 'main') {
        return false;
    }
    if (info.level > 31) {
        return false;
    }
    if (info.height > config.height) {
        return false;
    }
    if (get_subfile(info.filename)) {
        return false;
    }
    if (info.bitrate > 600000) {
        return false;
    }
    return true;
}

function get_video_opt(info) {
    if (can_copy_video(info)) {
        return ' -c:v copy ';
    }
    let templ = ' -c:v libx264 -crf %d -preset veryfast -profile:v main -level 3.1 -tune %s ';
    let cmd = util.format(templ, config.crf, config.tune);
    let vf = [];
    if (info.height > config.height) {
        let sar = info.width / info.height;
        let height = config.height;
        let width = Math.floor((height * sar) / 16) * 16;
        vf.push(util.format('scale=%d:%d', width, height))
    }
    let subfile = get_subfile(info.filename);
    if (subfile) {
        let ext = path.extname(subfile);
        if (ext == '.ass') {
            vf.push(util.format('ass="%s"', subfile));
        } else if (ext == ".srt") {
            vf.append(util.format('subtitles="%s"', subfile))
        }
    }
    if (vf.length > 0) {
        cmd = cmd + ' -vf ' + vf.join(',') + ' '
    }
    return cmd;
}

function get_audio_opt(info) {
    if (info.isavc && info.isaac && info.audio_bitrate <= 120) {
        return ' -c:a copy '
    } else {
        return ' -c:a libfaac -b:a 96k '
    }
}

function exec(cmd, pout, perr) {
    console.log('exec:', cmd);
    return new Promise((resolve, reject) => {
        let p = require('child_process').exec(cmd, (err, stdout, stderr) => {
            if (err) {
                console.log('failed:', cmd);
                reject(err);
            } else {
                console.log('finish:', cmd);
                resolve(stdout)
            }
        });
        if (pout) {
            p.stdout.pipe(pout);
        }
        if (perr) {
            p.stderr.pipe(perr);
        }
    });
}

function get_media_info(filename) {
    let ffprobe_cmd = util.format('ffprobe -v quiet -print_format json -show_format -show_streams "%s"', filename);
    return exec(ffprobe_cmd, null, null).then((stdout) => {
        let data = JSON.parse(stdout);
        let hasvideo = false;
        let hasaudio = false;
        let hassub = false;
        let info = {
            filename: filename
        }
        for (var stream of data.streams) {
            if (!hasvideo && stream.codec_type == 'video') {
                hasvideo = true
                if (stream.codec_name == 'h264') {
                    info.isavc = true;
                    info.profile = stream.profile.toLowerCase();
                    info.level = stream.level
                }
                info.width = stream.width;
                info.height = stream.height;
                info.bitrate = parseInt(stream.bit_rate);
                if (isNaN(info.bitrate)) {
                    info.bitrate = 9999999;
                }
            } else if (!hasaudio && stream.codec_type == 'audio') {
                hasaudio = true;
                if (stream.codec_name = 'aac') {
                    info.isaac = true;
                    info.audio_bitrate = Math.floor(parseInt(stream.bit_rate) / 1000);
                    if (isNaN(info.audio_bitrate)) {
                        info.audio_bitrate = 9999999;
                    }
                }
            } else if (!hassub && stream.codec_type == 'subtitle') {
                hassub = true;
                info.subindex = stream.index;
                info.sub = stream.codec_name;
            }
        }
        return info;
    });
}

function convert(filename) {
    get_media_info(filename).then((info) => {
        if ('sub' in info) {
            let subfile = util.format('%s.%s', filename, info.sub);
            let cmd = 'ffmpeg -i "%s" -an -vn -c:s:%d %s -n "%s"';
            cmd = util.format(cmd, filename, info.subindex, info.sub, subfile);
            return exec(cmd, process.stdout, null).then((stdout) => {
                return info;
            }).catch((err) => {
                if (err.message.indexOf('already exists') > -1) {
                    console.log('subtitle already exist');
                    return info;
                } else {
                    throw err;
                }
            });
        } else {
            return info;
        }
    }).then((info) => {
        let ext = path.extname(filename).toLowerCase();
        let dir = path.normalize(path.dirname(filename));
        let base = path.basename(filename, ext);

        let target = ''
        let target2 = ''
        if ((dir == '.' || dir == __dirname) && ext == '.mp4') {
            target = base + '.mp4cvt.mp4'
        } else {
            target = base + '.mp4'
        }
        let ffmpeg_cmd = 'ffmpeg -i "%s" -sn -metadata title="%s" %s %s -n "%s"';
        let cmd = util.format(ffmpeg_cmd, filename, base, get_audio_opt(info), get_video_opt(info), target);
        return cmd;
    }).then((cmd) => {
        return exec(cmd, process.stdout, process.stdout);
    }).catch((err) => {
        if (err.message.indexOf('already exists') > -1) {
            console.log('target file already exist');
        } else {
            console.log(err.message)
        }
    });
}

convert(process.argv[2]);
