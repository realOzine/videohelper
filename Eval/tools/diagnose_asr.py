"""Read-only capture of the two failed baseline ASR recordings; CPU FunASR only."""

import argparse
import copy
import hashlib
import json
import os
import subprocess
import sys
from importlib.metadata import version
from pathlib import Path

import environment
import yaml

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "ml"))


def write_json(path, value):
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n")


def violations(result, start_ms, end_ms):
    """Mirror adapter clipping while retaining every offending raw timestamp."""
    tokens = result["raw_text"].split()
    stamps = result["timestamp"]
    if len(tokens) != len(stamps):
        raise ValueError("token/timestamp count mismatch")
    invalid = []
    previous = start_ms
    for index, (token, (raw_start, raw_end)) in enumerate(zip(tokens, stamps, strict=True)):
        start = min(end_ms, max(start_ms, start_ms + int(raw_start)))
        end = min(end_ms, max(start_ms, start_ms + int(raw_end)))
        if not previous <= start < end:
            invalid.append({
                "token_index": index, "token": token,
                "raw_timestamp": [raw_start, raw_end],
                "clipped_timestamp": [start, end], "previous_end_ms": previous,
                "overlap_ms": max(0, previous - start),
                "zero_or_negative_duration": start >= end,
            })
        previous = end
    return invalid


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--state", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    state = json.loads(args.state.read_text())
    environment.runtime_env(state)  # Validate the isolated DB/bucket before read access.
    args.output.mkdir(parents=True, exist_ok=False)
    media_dir = ROOT / "Eval/.work/asr-diagnostic"
    media_dir.mkdir(parents=True, exist_ok=True)
    records = json.loads(environment._psql(
        "SELECT json_agg(row_to_json(v)) FROM (SELECT id,title,normalized_video_key,"
        "duration_ms,status FROM video WHERE id IN (3,4) ORDER BY id) v",
        database=state["db_name"],
    ))
    if [record["id"] for record in records] != [3, 4]:
        raise ValueError("expected baseline video IDs 3 and 4")
    client = environment.s3_client(state)
    for record in records:
        path = media_dir / f"{record['id']}.mp4"
        client.download_file(state["bucket"], record["normalized_video_key"], str(path))
        record["media_sha256"] = hashlib.sha256(path.read_bytes()).hexdigest()
    os.environ["HF_HUB_OFFLINE"] = "1"
    from app.adapters.funasr import FunASRTranscriber
    from app.api.errors import MLError
    from app.api.schemas import TranscriberInfo
    from app.core.fingerprint import fingerprint

    config = next(model for model in yaml.safe_load((ROOT / "ml/models.local.yaml").read_text())["models"]
                  if model["adapter"] == "funasr")
    info = TranscriberInfo(
        id=config["id"], languages=config["languages"],
        max_window_ms=config["max_window_ms"], fingerprint=fingerprint(config, fake=False),
    )
    instance = FunASRTranscriber(info, config)
    instance.load()
    generate = instance.model.generate
    inference = instance.model.inference
    summary = {"run_id": state["run_id"], "bucket": state["bucket"],
               "code_revision": subprocess.check_output(
                   ["git", "rev-parse", "HEAD"], cwd=ROOT, text=True).strip(),
               "funasr_version": version("funasr"),
               "adapter_sha256": hashlib.sha256(
                   (ROOT / "ml/app/adapters/funasr.py").read_bytes()).hexdigest(),
               "config": config, "fingerprint": info.fingerprint, "recordings": []}
    for record in records:
        raw_results = []
        stages = []

        def capture_stage(*positional, _stages=stages, _video_id=record["id"], **kwargs):
            results = inference(*positional, **kwargs)
            model = kwargs.get("model")
            if model is instance.model.vad_model or model is instance.model.model:
                _stages.append({
                    "stage": "vad" if model is instance.model.vad_model else "asr",
                    "results": copy.deepcopy(results),
                })
                write_json(args.output / f"video-{_video_id}-stages.json", _stages)
            return results

        def capture(_raw_results=raw_results, _video_id=record["id"], **kwargs):
            results = generate(**kwargs)
            _raw_results.extend(results)
            write_json(args.output / f"video-{_video_id}-raw.json", results)
            return results

        instance.model.generate = capture
        instance.model.inference = capture_stage
        try:
            instance.transcribe(media_dir / f"{record['id']}.mp4", "zh", 0, record["duration_ms"])
            record["adapter_error"] = None
        except MLError as exc:
            record["adapter_error"] = {"status": exc.status, "code": exc.code, "message": exc.message}
        if len(raw_results) != 1:
            raise ValueError("unexpected recording count")
        record["violations"] = violations(raw_results[0], 0, record["duration_ms"])
        record["token_count"] = len(raw_results[0]["timestamp"])
        summary["recordings"].append(record)
        write_json(args.output / "summary.json", summary)
        print(json.dumps(record, ensure_ascii=False), flush=True)


if __name__ == "__main__":
    main()
