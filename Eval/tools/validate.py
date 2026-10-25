#!/usr/bin/env python3
"""Check media integrity and label structure; this is not a retrieval-quality evaluation."""
import hashlib
import json
import subprocess
from collections import Counter
from pathlib import Path

import numpy as np

ROOT = Path(__file__).resolve().parents[1]


def require(condition, message):
    if not condition:
        raise ValueError(message)


def validate_annotations(manifest, queries):
    videos = {v["id"]: v for v in manifest["videos"]}
    require(len(videos) == len(manifest["videos"]), "duplicate media ID")
    require(len({q["id"] for q in queries}) == len(queries), "duplicate query ID")
    for q in queries:
        require(bool(q["query"].strip()), "empty query")
        require(type(q["answerable"]) is bool, "answerable must be boolean")
        require(q["scope"] == "corpus", "bootstrap queries must search the full corpus")
        require(q["answerable"] == bool(q["targets"]), "answerability/targets mismatch")
        require(q["category"] in ("visual", "speech", "mixed", "negative"), "unknown category")
        require((q["category"] == "negative") == (not q["answerable"]), "negative category mismatch")
        for target in q["targets"]:
            require(target["media_id"] in videos, "unknown target media")
            video = videos[target["media_id"]]
            scenes = {s["id"]: s for s in video["scenes"]}
            require(target["scene_id"] in scenes, "unknown target scene")
            scene = scenes[target["scene_id"]]
            s, e = target["start_ms"], target["end_ms"]
            require(type(s) is int and type(e) is int, "timestamps must be integer milliseconds")
            require(0 <= scene["start_ms"] <= s < e <= scene["end_ms"] <= video["duration_ms"], "target outside scene or invalid interval")
            jump = target["acceptable_jump_ms"]
            require(len(jump) == 2 and all(type(t) is int for t in jump), "invalid jump window")
            require(scene["start_ms"] <= jump[0] < jump[1] <= scene["end_ms"], "jump window outside scene")


def inspect_video(video):
    path = ROOT / video["path"]
    require(path.resolve().is_relative_to(ROOT), "media must remain inside Eval")
    with path.open("rb") as stream:
        require(hashlib.file_digest(stream, "sha256").hexdigest() == video["sha256"], "frozen video hash changed")
    info = json.loads(subprocess.check_output(["ffprobe", "-v", "error", "-show_streams", "-show_format", "-of", "json", str(path)]))
    streams = info["streams"]
    v = [s for s in streams if s["codec_type"] == "video"]
    a = [s for s in streams if s["codec_type"] == "audio"]
    require(len(v) == len(a) == 1 and len(streams) == 2, "expected one video and one audio stream, no embedded subtitles")
    require(v[0]["codec_name"] == "h264" and a[0]["codec_name"] == "aac", "unexpected media codecs")
    require((v[0]["width"], v[0]["height"], v[0]["pix_fmt"]) == (1280,720,"yuv420p"), "unexpected dimensions/pixel format")
    require(v[0]["r_frame_rate"] == "24/1", "unexpected frame rate")
    duration = float(info["format"]["duration"])
    require(abs(duration*1000-video["duration_ms"]) <= 50, "duration mismatch")
    require(int(v[0]["nb_frames"]) == round(video["duration_ms"]*24/1000), "frame count mismatch")
    subprocess.run(["ffmpeg", "-v", "error", "-xerror", "-i", str(path), "-f", "null", "-"], check=True, stdout=subprocess.DEVNULL)
    pcm = subprocess.check_output(["ffmpeg", "-v", "error", "-i", str(path), "-map", "0:a:0", "-ac", "1", "-ar", "16000", "-f", "f32le", "-"])
    samples = np.frombuffer(pcm,dtype="<f4")
    require(np.isfinite(samples).all() and np.max(np.abs(samples)) > 0.1, "invalid or inaudible audio")
    require(np.max(np.abs(samples)) < 0.99, "audio clipping")
    end = 0
    for scene in video["scenes"]:
        require(scene["start_ms"] == end, "noncontiguous scene timeline")
        end = scene["end_ms"]
        for u in scene["utterances"]:
            require(scene["start_ms"] <= u["start_ms"] < u["end_ms"] <= end, "speech crosses scene boundary")
            segment = samples[round(u["start_ms"]*16):round(u["end_ms"]*16)]
            require(len(segment)>0 and float(np.sqrt(np.mean(segment**2)))>0.01, "missing narration in expected window")
    require(end == video["duration_ms"], "scene timeline does not cover video")
    return {"id":video["id"],"duration_seconds":duration,"frames":int(v[0]["nb_frames"]),"bytes":path.stat().st_size,"full_decode":"passed","sha256":"matched","audio_peak":round(float(np.max(np.abs(samples))),4),"audio_rms":round(float(np.sqrt(np.mean(samples**2))),4)}


def main():
    manifest = json.loads((ROOT/"manifest.json").read_text())
    queries = [json.loads(line) for line in (ROOT/"annotations/queries.jsonl").read_text().splitlines() if line]
    validate_annotations(manifest, queries)
    require(len(manifest["videos"]) == 5, "expected five videos")
    counts = Counter(q["category"] for q in queries)
    require(counts == {"visual":10,"speech":10,"mixed":10,"negative":5}, "expected 30 positive and five negative queries")
    checked = []
    for video in manifest["videos"]:
        checked.append(inspect_video(video))
        print(f"validated {video['id']}",flush=True)
    report = {"dataset_id":manifest["dataset_id"],"status":"passed","scope":"file hashes, full media decode, audio signal, structural and temporal label checks","not_verified":["independent human annotation","full audio intelligibility review","real-model retrieval quality","real action recognition"],"query_counts":dict(counts),"videos":checked,"total_duration_seconds":sum(v["duration_seconds"] for v in checked)}
    (ROOT/"review/validation.json").write_text(json.dumps(report,ensure_ascii=False,indent=2)+"\n")
    print(json.dumps({"status":"passed","videos":len(checked),"queries":len(queries),"seconds":report["total_duration_seconds"]}))


if __name__ == "__main__":
    main()
