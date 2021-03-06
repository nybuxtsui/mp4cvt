#!/usr/bin/env python

import sys, optparse, subprocess, re, os, shutil, time

g_tmp = '/Volumes/ramdisk'
g_tune = 'animation'

parser = optparse.OptionParser()
parser.add_option('-a', '--action',
                  dest='action', choices=['cvt', 'info', 'cat', 'extract'], default='cvt')
parser.add_option('-i', '--input',
                  dest='input', metavar='FILE')
parser.add_option('-t', '--tune',
                  dest='tune', choices=['animation', 'film'], default='animation')
parser.add_option('-f', '--format',
                  dest='format', choices=['mkv', 'mp4'], default='mp4')
parser.add_option('-c', '--aaccopy',
                  dest='aaccopy', choices=['on', 'off', 'auto'], default='off')
parser.add_option('-o', '--options',
                  dest='options', default='')
parser.add_option('-p', '--program',
                  dest='program', default='f', choices=['m', 'f', 'u'])
parser.add_option('-r', '--track', default='2', dest='track')
parser.add_option('-u', '--update', default=False, dest='update', action='store_true')
parser.add_option('-s', '--height', default='480', dest='height')
parser.add_option('-q', '--crf', default='26', dest='crf')
(options, args) = parser.parse_args()

g_height = int(options.height)
def get_media_info(f):
    try:
        p = subprocess.Popen(['mediainfo', '-f', f], stdout=subprocess.PIPE)
    except OSError:
        print 'mediainfo not found'
        exit(1)
    p.wait()
    ret = p.returncode
    if ret != 0:
        print 'input is not mediafile'
        exit(1)
    out = p.communicate()[0]
    out = re.compile(' ').sub('', out)
    #print out
    info = {}
    isaudio = False
    info['filename'] = f
    info['isavc'] = False
    info['isaac'] = False
    info['fps'] = 0
    info['audio_bitrate'] = 0
    for l in out.split('\n'):
        if l == 'Format:AVC':
            info['isavc'] = True
        elif not isaudio and l[0:14] == 'Formatprofile:':
            p = l[14:].upper().split("@")
            info['h264profile'] = p[0]
            if len(p) > 1 and p[1][0] == 'L':
                info['h264level'] = int(float(p[1][1:]) * 10)
        elif l[0:6] == 'Width:' and not info.has_key('width'):
            info['width'] = int(l[6:])
        elif l[0:7] == 'Height:' and not info.has_key('height'):
            info['height'] = int(l[7:])
        elif l[0:10] == 'Framerate:' and not info.has_key('fps'):
            info['fps'] = float(l[10:])
        elif l[0:9] == 'Duration:' and not info.has_key('duration'):
            info['duration'] = int(l[9:]) / 1000
        elif l == 'Video':
            isaudio = False
        elif l == 'Audio':
            isaudio = True
        elif l == 'Format:AAC':
            info['isaac'] = True
        elif isaudio and l[0:8] == 'Bitrate:':
            info['audio_bitrate'] = float(l[8:-4])
    return info

def extrace_mkv_ass(f):
    name = os.path.splitext(f)[0]
    cmd = 'mkvextract tracks %s.mkv %s:%s.ass' % (name, options.track, name)
    print cmd
    os.system(cmd)

def get_subfile(info):
    name = os.path.splitext(info['filename'])[0]
    if os.path.exists(name + ".ass"):
        return name + ".ass"
    elif os.path.exists(name + ".srt"):
        return name + ".srt"
    else:
        name = os.path.splitext(os.path.basename(info['filename']))[0]
        if os.path.exists(name + ".ass"):
            return name + ".ass"
        elif os.path.exists(name + ".srt"):
            return name + ".srt"
        else:
            return None

def can_copy_video(info):
    if not info['isavc']:
        return False
    if info['h264profile'] != 'HIGH' and info['h264profile'] != 'MAIN':
        return False
    if info['h264level'] > 31:
        return False
    if info['height'] > g_height:
        return False
    if get_subfile(info) is not None:
        return False
    return True

def convert_video(info):
    cmd = 'mencoder "%s" -ovc x264 -x264encopts preset=veryfast:crf=%s:tune=%s:profile=main:level=31 -nosound ' % (info['filename'], options.crf, options.tune)
    width = info['width']
    height = info['height']
    cmd = cmd + ' -ofps 24000/1001 '
    if (height > g_height):
        sar = width / float(height)
        height = g_height
        width = int(height * sar) / 16 * 16
        cmd = cmd + '-sws 9 -vf scale=%d:%d,harddup ' % (width, height)
    else:
        cmd = cmd + ' -vf harddup '
    subfile = get_subfile(info)
    if subfile is not None:
        cmd = cmd + ' -sub "%s" -subcp utf-8 -spuaa 2 -subfont-text-scale 5 -ass -embeddedfonts ' % (subfile)
    cmd = cmd + ' -of rawvideo -o %s/a.h264 ' % (g_tmp)
    print cmd
    os.system(cmd)

def convert_audio(info):
    cmd = 'ffmpeg -i "%s" ' % (info['filename'])
    if info['isaac']:
        if options.aaccopy == 'on':
            cmd = cmd + ' -c:a copy '
        elif options.aaccopy == 'off':
            cmd = cmd + ' -c:a libfaac -ac 2 -b:a 96k '
        else: # aaccopy == 'auto'
            if info['audio_bitrate'] == 0: 
                os.system('ffmpeg -i "%s" -vn -c:a copy -y %s/a.aac' % (info['filename'], g_tmp))
                bitrate = os.path.getsize('%s/a.aac' % (g_tmp)) / info['duration'] * 8 / 1024
                os.remove('%s/a.aac' % (g_tmp))
                if bitrate > 120:
                    cmd = cmd + ' -c:a libfaac -ac 2 -b:a 96k '
                else:
                    cmd = cmd + ' -c:a copy '
            elif info['audio_bitrate'] > 120:
                cmd = cmd + ' -c:a libfaac -ac 2 -b:a 96k '
            else:
                cmd = cmd +  ' -c:a copy '
    else:
        cmd = cmd + ' -c:a libfaac -ac 2 -b:a 96k '
    cmd = cmd + ' -y %s/a.aac' % (g_tmp)
    os.system(cmd)


def get_video_opt(info):
    if can_copy_video(info):
        return ' -c:v copy '
    else:
        cmd = ''
        #if info['fps'] == 0 or info['fps'] > 23.98:
            #cmd = cmd + ' -r 24000/1001 -vsync 1'
        cmd = cmd + ' -c:v libx264 -crf %s -preset veryfast -profile:v main -level 3.1 -tune %s ' % (options.crf, options.tune)
        width = info['width']
        height = info['height']
        vf = []
        if (height > g_height):
            sar = width / float(height)
            height = g_height
            width = int(height * sar) / 16 * 16
            vf.append('scale=%d:%d' % (width, height))
        subfile = get_subfile(info)
        if subfile is not None:
            subext = os.path.splitext(subfile)[1]
            if subext == '.ass':
                vf.append('ass="%s"' % (subfile))
            elif subext == '.srt':
                vf.append('subtitles="%s"' % (subfile))
        if len(vf) > 0:
            cmd = cmd + ' -vf ' + ','.join(vf) + ' '
        return cmd

def get_audio_opt(info):
    if info['isaac']:
        if options.aaccopy == 'on':
            return ' -c:a copy '
        elif options.aaccopy == 'off':
            return ' -c:a libfaac -b:a 96k '
        else: # aaccopy == 'auto'
            if info['audio_bitrate'] == 0:
                os.system('ffmpeg -i "%s" -vn -c:a copy -y %s/a.aac' % (info['filename'], g_tmp))
                bitrate = os.path.getsize('%s/a.aac' % (g_tmp)) / info['duration'] * 8 / 1024
                os.remove('%s/a.aac' % (g_tmp))
                if bitrate > 120:
                    return ' -c:a libfaac -b:a 96k '
                else:
                    return ' -c:a copy '
            elif info['audio_bitrate'] > 120:
                return ' -c:a libfaac -b:a 96k '
            else:
                return ' -c:a copy '
    else:
        return ' -c:a libfaac -b:a 96k '

def concat(file):
    dir = os.path.dirname(file)
    if dir == '':
        dir = '.'
    if not os.path.isabs(dir):
        dir = os.path.abspath(dir)
    base = os.path.basename(file)
    out = os.path.splitext(base)[0] + "." + options.format
    if os.path.exists(out):
        print 'target file exist'
        exit(1)

    r = re.compile('^' + base)
    l = []
    for f in os.listdir(dir):
        if r.match(f) is not None:
            l.append(os.path.join(dir, f))
    l.sort()
    for f in l:
        print f
    if len(l) != 0:
        f = open('%s/concat.txt' % (g_tmp), 'w')
        for i in l:
            f.write('file \'%s\'\n' % (i))
        f.close()
        cmd = 'ffmpeg -y -f concat -i %s/concat.txt -c copy %s/a.%s' % (g_tmp, g_tmp, options.format)
        print cmd
        os.system(cmd)
        shutil.move(g_tmp + '/a.' + options.format, out)

if options.action is None:
    print 'action miss'
    exit(1)
if options.input is None:
    print 'input miss'
    exit(1)
if options.action == 'info':
    print get_media_info(options.input)
    exit(0)
if options.action == 'cat':
    concat(options.input)
    exit(0)
if options.action == 'extract':
    extrace_mkv_ass(options.input)
    exit(0)
if options.action == 'cvt':
    name = os.path.splitext(os.path.basename(options.input))[0]
    out = name + "." + options.format
    if os.path.exists(out):
        print 'target file exist'
        exit(1)

    info = get_media_info(options.input)
    if options.program == 'f':
        cmd = 'ffmpeg -i "%s" -sn -metadata title="%s" %s ' % (info['filename'], name, options.options)
        cmd = cmd + get_audio_opt(info)
        cmd = cmd + get_video_opt(info)
        cmd = cmd + ' -y ' + g_tmp + '/a.' + options.format
        print cmd
        os.system(cmd)
        shutil.move(g_tmp + '/a.' + options.format, out)
    elif options.program == 'u':
        cmd = 'ffmpeg -i "%s" -sn -metadata title="%s" -codec copy %s ' % (info['filename'], name, options.options)
        cmd = cmd + ' -y ' + g_tmp + '/a.' + options.format
        print cmd
        os.system(cmd)
        shutil.move(g_tmp + '/a.' + options.format, out)
    else:
        convert_audio(info)
        convert_video(info)
        os.system('MP4Box -fps 23.976 -add %s/a.h264 -add %s/a.aac %s' % (g_tmp, g_tmp, out))
    exit(0)
