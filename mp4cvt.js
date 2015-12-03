#!/usr/bin/env node
'use strict';

let exec = require('child_process').exec;
let path = require('path'); 
let fs = require('fs');
let util = require('util');

b = 3

let config = {
    height: 480,
    crf: 26,
    tune: 'animation',
    tmpdir: '.'
}

function get_audio_option(info) {
    if (info.isaac && info.audio_bitrate > 120) {
        return ' -c:a libfaac -b:a 96k '
    } else {
        return ' -c:a copy '
    }
}

function get_subfile(filename) {
    let subfile = filename + '.srt';
    if (fs.existsSync(subfile)) {
        return subfile;
    }
    subfile = filename + '.srt';
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
        console.log("aa")
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
    return true;
}

function get_video_option(info) {
    if (can_copy_video(info)) {
        return ' -c:v copy ';
    }
    let templ = ' -c:v libx264 -crf %d -preset veryfast -profile:v main -level 3.1 -tune %s ';
    let cmd = util.format(templ, config.crf, config.tune);
    let vf = [];
    if (info.height > config.height) {
        let sar = width / height;
        height = config.height;
        width = Math.floor((height * sar) / 16 * 16);
        vf.push(util.format('scale=%d:%d', width, height))
    }
    let subfile = get_subfile(info);
    if (subfile) {
        let ext = path.extname(subfile);
        if (ext == '.ass') {
            vf.push('ass="{}"', subfile);
        } else if (ext == ".srt") {
            vf.append(util.format('subtitles="%s"', subfile))
        }
    }
    if (vf.length > 0) {
        cmd = cmd + ' -vf ' + vf.join(',') + ' '
    }
    return cmd;
}

function get_media_info(filename, callback) {
    exec('ffprobe -v quiet -print_format json -show_format -show_streams ' + filename, function(err, stdout, stderr) {
        if (err) throw err;
        let data = JSON.parse(stdout);
        let hasvideo = false;
        let hasaudio = false;
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
            } if (!hasaudio && stream.codec_type == 'audio') {
                hasaudio = true;
                if (stream.codec_name = 'aac') {
                    info.isaac = true;
                    info.audio_bitrate = Math.floor(parseInt(stream.bit_rate) / 1000);
                }
            }
        }
        //console.log(info);
        callback(info);
    })
}

function convert(filename) {
    get_media_info(filename, function(info) {
        let ext = path.extname(filename);
        let dir = path.dirname(filename);
        let base = path.basename(filename, ext);

        let cmd = util.format('ffmpeg -i "%s" -sn -metadata title="%s" ', filename, base);
        cmd += get_audio_option(info);
        cmd += get_video_option(info);
        cmd += util.format(' -y %s', path.join(config.tmpdir, 'a.mp4'));

        console.log(cmd);
    });
}

convert('c:\\users\\xubin\\c01.mp4')
