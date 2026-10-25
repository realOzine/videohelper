#!/usr/bin/env python3
"""Build the frozen synthetic corpus. Uses installed Pillow/numpy, ffmpeg and macOS say."""
from __future__ import annotations

import hashlib
import html
import json
import math
import platform
import subprocess
import sys
import wave
from pathlib import Path

import numpy as np
from PIL import Image, ImageDraw, ImageFont

ROOT = Path(__file__).resolve().parents[1]
WORK = ROOT / ".work"
SR = 48000
FONT = "/System/Library/Fonts/Supplemental/Arial.ttf"


def run(args):
    subprocess.run([str(x) for x in args], check=True, stdout=subprocess.DEVNULL)


def digest(path):
    with Path(path).open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def probe(path):
    return json.loads(subprocess.check_output([
        "ffprobe", "-v", "error", "-show_streams", "-show_format", "-of", "json", str(path)
    ]))


def write_json(path, data):
    Path(path).write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n")


def write_wav(path, samples):
    assert np.isfinite(samples).all() and np.max(np.abs(samples)) < 1, "invalid audio"
    with wave.open(str(path), "wb") as stream:
        stream.setnchannels(1)
        stream.setsampwidth(2)
        stream.setframerate(SR)
        stream.writeframes((samples * 32767).astype("<i2").tobytes())


def diagram(number, output):
    """Code-native teaching diagrams; the six topics are explicitly drawn, not AI text."""
    im = Image.new("RGB", (1280, 720), "#101c31")
    d = ImageDraw.Draw(im)
    blue, gold, muted = "#3b9fea", "#efc66b", "#7189a9"
    font = lambda size: ImageFont.truetype(FONT, size)
    titles = ["A SHARED LEDGER", "LINKED BLOCKS", "A CHANGED RECORD", "AGREEING ON HISTORY", "PRIVATE KEYS", "VERIFY THE SOURCE"]
    d.text((70, 52), "FIELD NOTES  /  BLOCKCHAIN", font=font(18), fill=muted)
    d.text((70, 94), titles[number], font=font(43), fill="#e9eff7")
    d.line((70, 165, 1210, 165), fill="#29405e", width=2)

    def card(box, color=blue, label=""):
        d.rounded_rectangle(box, radius=18, fill="#172d49", outline=color, width=5)
        if label:
            cx, cy = (box[0]+box[2])/2, (box[1]+box[3])/2
            d.text((cx, cy), label, font=font(35), fill=color, anchor="mm")

    def arrow(a, b, color=gold):
        d.line((*a, *b), fill=color, width=6)
        angle = math.atan2(b[1]-a[1], b[0]-a[0])
        points = [b] + [(b[0]-18*math.cos(angle+off), b[1]-18*math.sin(angle+off)) for off in (-0.5,0.5)]
        d.polygon(points, fill=color)

    def ledger(cx, cy):
        card((cx-90,cy-100,cx+90,cy+100),gold)
        for row in range(5):
            d.line((cx-55,cy-65+row*32,cx+55,cy-65+row*32),fill=gold,width=4)

    if number in (0,3):
        points = [(640+410*math.cos(2*math.pi*i/(6 if number==0 else 5)),414+195*math.sin(2*math.pi*i/(6 if number==0 else 5))) for i in range(6 if number==0 else 5)]
        for x,y in points:
            d.line((x,y,640,414),fill="#446080",width=4)
        ledger(640,414)
        for i,(x,y) in enumerate(points):
            card((x-60,y-42,x+60,y+42),blue)
            d.line((x-34,y+55,x+34,y+55),fill=blue,width=6)
            if number==3:
                color = "#62d9a0" if i<4 else "#f0786e"
                if i<4:
                    d.line((x-18,y,x-3,y+14,x+23,y-18),fill=color,width=6)
                else:
                    d.line((x-16,y-16,x+16,y+16),fill=color,width=6)
                    d.line((x-16,y+16,x+16,y-16),fill=color,width=6)
    elif number in (1,2):
        for i,x in enumerate((245,640,1035)):
            color = "#f17b57" if number==2 and i==1 else blue
            card((x-125,290,x+125,510),color,f"0{i+1}")
            d.text((x,550),"HASH ?" if number==2 and i==1 else "HASH",font=font(22),fill=color,anchor="mm")
            if i<2:
                arrow((x+145,400),(x+245,400))
    elif number==4:
        d.ellipse((185,285,365,465),outline=gold,width=26)
        d.line((350,375,570,375),fill=gold,width=28)
        d.line((510,375,510,435),fill=gold,width=20)
        d.line((554,375,554,422),fill=gold,width=20)
        d.arc((815,250,1025,490),180,360,fill=blue,width=23)
        d.rounded_rectangle((785,360,1055,565),radius=24,fill=blue)
        d.ellipse((904,422,936,454),fill="#101c31")
        d.rectangle((914,445,926,492),fill="#101c31")
    else:
        ledger(300,397)
        ledger(980,397)
        d.text((640,385),"?",font=font(125),fill=gold,anchor="mm")
        d.ellipse((577,514,657,594),outline=gold,width=9)
        d.line((647,584,693,630),fill=gold,width=12)
    d.text((70,666),f"0{number+1}  /  06",font=font(18),fill=muted)
    im.save(output)


def narration(video, scene, index, blueprint):
    folder = ROOT / "assets/voice" / video["id"]
    folder.mkdir(exist_ok=True)
    total = np.zeros(round(video["scene_seconds"]*SR), dtype=np.float32)
    cursor = round(1.0*SR)
    utterances = []
    for part, utterance in enumerate(scene["utterances"], 1):
        settings = blueprint["voices"][utterance["lang"]]
        stem = folder / f"s{index+1}_{part}"
        textfile = stem.with_suffix(".txt")
        aiff, wav = stem.with_suffix(".aiff"), stem.with_suffix(".wav")
        textfile.write_text(utterance["text"] + "\n")
        run(["say","-v",settings["name"],"-r",settings["rate"],"-f",textfile,"-o",aiff])
        run(["ffmpeg","-hide_banner","-loglevel","error","-y","-i",aiff,"-ac","1","-ar",SR,"-c:a","pcm_s16le",wav])
        with wave.open(str(wav),"rb") as stream:
            samples = np.frombuffer(stream.readframes(stream.getnframes()),dtype="<i2").astype(np.float32)/32768
        peak = np.max(np.abs(samples))
        assert peak > 0.001, f"silent voice: {scene['id']}"
        samples *= 0.62 / peak
        end = cursor + len(samples)
        assert end <= len(total)-int(0.5*SR), f"narration overflows scene: {scene['id']} ({end/SR:.2f}s)"
        total[cursor:end] += samples
        base = index*video["scene_seconds"]*1000
        utterances.append({"lang":utterance["lang"],"text":utterance["text"],"start_ms":base+round(cursor/SR*1000),"end_ms":base+round(end/SR*1000),"source":str(wav.relative_to(ROOT))})
        cursor = end + round(0.35*SR)
    return total, utterances


def music(seconds, root_note, seed):
    rng = np.random.default_rng(seed)
    output = np.zeros(round(seconds*SR),dtype=np.float32)
    # Original deterministic arpeggios: soft additive synthesis, not a recording of an instrument.
    for beat, start in enumerate(np.arange(0,seconds,0.75)):
        offsets = (0,4,7,12,7,4,2,7)
        note = root_note + offsets[beat%len(offsets)]
        t = np.arange(min(round(2.8*SR),len(output)-round(start*SR)),dtype=np.float32)/SR
        frequency = 440*2**((note-69)/12)
        phase = rng.uniform(0,2*np.pi)
        tone = (np.sin(2*np.pi*frequency*t+phase)+0.22*np.sin(4*np.pi*frequency*t))*np.exp(-2.1*t)*(1-np.exp(-70*t))
        begin = round(start*SR)
        output[begin:begin+len(tone)] += 0.028*tone
    fade = min(SR*2,len(output)//2)
    output[:fade] *= np.linspace(0,1,fade)
    output[-fade:] *= np.linspace(1,0,fade)
    return output


def render_scene(video,index,image,output):
    frames = video["scene_seconds"]*24
    if video["sheet"]=="blockchain":
        crop=""
    else:
        w,h=Image.open(image).size
        ys=[0,384,740,h] if video["sheet"]=="study" else [0,round(h/3),round(h*2/3),h]
        left=round((index%2)*w/2)
        right=round((index%2+1)*w/2)
        row=index//2
        crop=f"crop={right-left-4}:{ys[row+1]-ys[row]-4}:{left+2}:{ys[row]+2},"
    vf=crop+f"scale=1920:1080:force_original_aspect_ratio=increase,crop=1920:1080,zoompan=z='1+0.045*on/{frames}':x='iw/2-iw/zoom/2':y='ih/2-ih/zoom/2':d={frames}:s=1280x720:fps=24,format=yuv420p"
    run(["ffmpeg","-hide_banner","-loglevel","error","-y","-filter_threads","1","-i",image,"-vf",vf,"-frames:v",frames,"-c:v","libx264","-preset","veryfast","-crf","21","-threads","2","-g","24","-an",output])


def timestamp(ms):
    return f"{ms//3600000:02}:{ms//60000%60:02}:{ms//1000%60:02},{ms%1000:03}"


def build():
    if (ROOT/"manifest.json").exists():
        raise SystemExit("Corpus is frozen. Run tools/validate.py; create a new version to change corpus bytes.")
    blueprint=json.loads((ROOT/"annotations/blueprint.json").read_text())
    manifest={"dataset_id":blueprint["dataset_id"],"version":1,"kind":"synthetic_bootstrap","fps":24,"resolution":[1280,720],"label_status":"script-derived; assistant frame review; independent human review pending","build_platform":platform.platform(),"blueprint_sha256":digest(ROOT/"annotations/blueprint.json"),"generator_sha256":digest(__file__),"videos":[]}
    queries=[]
    for vi,video in enumerate(blueprint["videos"]):
        print(f"Building {video['id']} ({video['scene_seconds']*6}s)",flush=True)
        work=WORK/video["id"]
        work.mkdir(parents=True,exist_ok=True)
        voiceparts,scenes,clips=[],[],[]
        for i,scene in enumerate(video["scenes"]):
            samples,utterances=narration(video,scene,i,blueprint)
            voiceparts.append(samples)
            image=ROOT/"assets/images"/(video["sheet"]+"-sheet.png")
            if video["sheet"]=="blockchain":
                image=ROOT/"assets/images"/f"blockchain-{i+1}.png"
                diagram(i,image)
            clip=work/f"scene-{i+1}.mp4"
            render_scene(video,i,image,clip)
            clips.append(clip)
            start=i*video["scene_seconds"]*1000
            end=start+video["scene_seconds"]*1000
            scenes.append({"id":scene["id"],"start_ms":start,"end_ms":end,"visual":scene["visual"],"utterances":utterances,"visual_asset":str(image.relative_to(ROOT))})
            category=scene["query"]["category"]
            target_start,target_end=(start,end) if category=="visual" else (utterances[0]["start_ms"],utterances[-1]["end_ms"])
            queries.append({"id":f"q{len(queries)+1:03}","query":scene["query"]["text"],"category":category,"scope":"corpus","answerable":True,"targets":[{"media_id":video["id"],"scene_id":scene["id"],"start_ms":target_start,"end_ms":target_end,"acceptable_jump_ms":[max(start,target_start-1000),min(end,target_start+2000)]}],"evidence":{"visual":scene["visual"],"spoken":[u["text"] for u in utterances]},"provenance":"synthetic_script_and_rendered_timeline","human_reviewed":False})
            print(f"  scene {i+1}/6: voice {sum(u['end_ms']-u['start_ms'] for u in utterances)/1000:.1f}s",flush=True)
        duration=video["scene_seconds"]*6
        speech=np.concatenate(voiceparts)
        soundtrack=music(duration,[60,50,67,57,52][vi],blueprint["seed"]+vi)
        voice_path=ROOT/"assets/voice"/(video["id"]+".wav")
        music_path=ROOT/"assets/music"/(video["id"]+".wav")
        mix_path=work/"mix.wav"
        write_wav(voice_path,speech)
        write_wav(music_path,soundtrack)
        write_wav(mix_path,speech+soundtrack)
        concat=work/"concat.txt"
        concat.write_text("".join(f"file '{p.name}'\n" for p in clips))
        output=ROOT/"videos"/(video["id"]+".mp4")
        run(["ffmpeg","-hide_banner","-loglevel","error","-y","-f","concat","-safe","1","-i",concat,"-i",mix_path,"-map","0:v:0","-map","1:a:0","-c:v","copy","-c:a","aac","-b:a","160k","-ar",SR,"-t",duration,"-movflags","+faststart",output])
        record={k:video[k] for k in ("id","title","genre","language")}
        record.update({"path":str(output.relative_to(ROOT)),"duration_ms":duration*1000,"sha256":digest(output),"bytes":output.stat().st_size,"voice_path":str(voice_path.relative_to(ROOT)),"music_path":str(music_path.relative_to(ROOT)),"scenes":scenes})
        manifest["videos"].append(record)
        subtitles=[]
        for scene in scenes:
            for u in scene["utterances"]:
                subtitles.append(f"{len(subtitles)+1}\n{timestamp(u['start_ms'])} --> {timestamp(u['end_ms'])}\n{u['text']}\n")
        (ROOT/"annotations"/(video["id"]+".srt")).write_text("\n".join(subtitles))
    for negative in blueprint["negatives"]:
        queries.append({"id":f"q{len(queries)+1:03}","query":negative["query"],"category":"negative","scope":"corpus","answerable":False,"targets":[],"evidence":{"absence_reason":negative["reason"]},"provenance":"synthetic_script_and_frame_inventory","human_reviewed":False})
    (ROOT/"annotations/queries.jsonl").write_text("".join(json.dumps(q,ensure_ascii=False)+"\n" for q in queries))
    write_json(ROOT/"manifest.json",manifest)
    review(manifest,queries)
    print("Corpus rendered. Run Eval/tools/validate.py before using it.",flush=True)


def review(manifest,queries):
    cards=[]
    for video in manifest["videos"]:
        frames=[]
        for i,scene in enumerate(video["scenes"]):
            target=ROOT/"review"/f"{video['id']}-{i+1}.jpg"
            run(["ffmpeg","-hide_banner","-loglevel","error","-y","-ss",(scene["start_ms"]+scene["end_ms"])/2000,"-i",ROOT/video["path"],"-frames:v","1","-q:v","2",target])
            frames.append(target)
        contact=Image.new("RGB",(960,870),"#101820")
        d=ImageDraw.Draw(contact)
        for i,path in enumerate(frames):
            x,y=(i%2)*480,(i//2)*290
            contact.paste(Image.open(path).resize((480,270)),(x,y))
            d.text((x+10,y+273),f"{video['id']} / scene {i+1} / {video['scenes'][i]['start_ms']/1000:.0f}s",font=ImageFont.truetype(FONT,13),fill="white")
        contact.save(ROOT/"review"/(video["id"]+"-contact.jpg"),quality=90)
        matching=[q for q in queries if q["answerable"] and q["targets"][0]["media_id"]==video["id"]]
        buttons="".join(f'<button type="button" data-video="{video["id"]}" data-seek="{q["targets"][0]["start_ms"]/1000}"><b>{q["id"]} · {q["category"]}</b>{html.escape(q["query"])}<small>{q["targets"][0]["start_ms"]/1000:.2f} – {q["targets"][0]["end_ms"]/1000:.2f}s</small></button>' for q in matching)
        cards.append(f'<section><div class="heading"><span>{html.escape(video["genre"])} / {video["language"]}</span><h2>{html.escape(video["title"])}</h2><p>{video["duration_ms"]/1000:.0f} 秒 · {video["id"]}</p></div><video id="{video["id"]}" controls preload="metadata" poster="{video["id"]}-contact.jpg" src="../{video["path"]}"></video><div class="queries">{buttons}</div></section>')
    negatives="".join(f'<li><b>{q["id"]}</b> {html.escape(q["query"])}</li>' for q in queries if not q["answerable"])
    page='''<!doctype html><html lang="zh-CN"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>VideoHelper · 合成评估视频库 v1</title><style>
*{box-sizing:border-box}body{margin:0;background:#f2f1ec;color:#18252b;font-family:system-ui,sans-serif;line-height:1.6}main{max-width:1120px;margin:auto;padding:48px 24px}header{padding:20px 0 36px}header small{letter-spacing:.18em;color:#496963}h1{font-size:40px;line-height:1.25;margin:12px 0}h2{margin:4px 0;font-size:26px}header p{max-width:780px;color:#536369}.note{background:#e0e8de;padding:16px 20px;border-radius:12px}section{background:white;border:1px solid #d9dfd9;border-radius:18px;padding:26px;margin:28px 0}.heading span{font-size:13px;color:#547268}.heading p{margin-top:4px;color:#647272}video{width:100%;max-height:570px;background:#101820;border-radius:10px}.queries{display:grid;grid-template-columns:1fr 1fr;gap:10px;margin-top:18px}button{font:inherit;text-align:left;border:1px solid #dce3df;border-radius:9px;background:#f7f9f6;padding:14px;cursor:pointer;color:inherit}button:hover,button:focus-visible{background:#e7efe5;outline:2px solid #476c5d}button b,button small{display:block}button b{font-size:12px;color:#46675b;margin-bottom:5px}button small{color:#718079;margin-top:6px}li{padding:7px}footer{color:#647272;font-size:13px}@media(max-width:650px){main{padding:24px 14px}h1{font-size:29px}.queries{grid-template-columns:1fr}section{padding:16px}}
</style><main><header><small>VIDEOHELPER / EVAL / SYNTHETIC V1</small><h1>五种内容，一份可回看的基线</h1><p>5 个原创合成视频 · 30 条有答案查询 · 5 条无答案查询。点击描述跳到标注位置；声音和字幕来源记录在 annotations/。此页可直接离线打开。</p><div class="note">标注来自合成脚本与实际时间线，独立人工复核尚未完成。生成分镜使用镜头平移缩放，不代表真实动作视频；原片没有烧录字幕。</div></header>'''+"".join(cards)+f'<section><h2>无答案查询</h2><p>这些内容不在本视频库中。当前 Top-K 返回结果不等于系统已实现“无答案拒绝”。</p><ol>{negatives}</ol></section>'+'''<footer>合成素材：内置 ImageGen、代码绘制图示、macOS 系统语音、原创程序合成配乐。未调用 VideoHelper 模型进行检索评分。</footer></main><script>document.querySelectorAll('button[data-video]').forEach(b=>b.addEventListener('click',()=>{document.querySelectorAll('video').forEach(v=>v.pause());const v=document.getElementById(b.dataset.video);v.currentTime=Number(b.dataset.seek);v.play().catch(()=>{});v.scrollIntoView({behavior:'smooth',block:'center'});}));</script></html>'''
    (ROOT/"review/index.html").write_text(page)


if __name__=="__main__":
    build()
