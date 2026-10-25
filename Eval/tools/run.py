"""Real indexing acceptance and frozen-index retrieval regression, as separate runs."""
import argparse
import fcntl
import hashlib
import json
import math
import os
import selectors
import signal
import subprocess
import time
from contextlib import contextmanager
from datetime import UTC, datetime
from pathlib import Path

import environment
import yaml

ROOT = Path(__file__).resolve().parents[1]
REPO = ROOT.parent


def digest(path):
    with Path(path).open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def read_json(path):
    return json.loads(Path(path).read_text())


def write_json(path, value):
    path = Path(path)
    temporary = path.with_suffix(path.suffix + ".tmp")
    temporary.write_text(json.dumps(value, ensure_ascii=False, indent=2, allow_nan=False) + "\n")
    temporary.replace(path)


def revision():
    return subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=REPO, text=True).strip()


def fresh(parent, name):
    environment.names(name)
    directory = parent / name
    directory.mkdir(parents=True, exist_ok=False)
    return directory


@contextmanager
def operation_lock():
    runtime = ROOT / ".runtime"
    runtime.mkdir(exist_ok=True)
    with (runtime / "operation.lock").open("a") as lock:
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            raise RuntimeError("another Eval index/restore operation is running") from None
        yield


def corpus():
    from validate import validate_annotations

    manifest = read_json(ROOT / "manifest.json")
    queries = [json.loads(line) for line in (ROOT / "annotations/queries.jsonl").read_text().splitlines() if line]
    validate_annotations(manifest, queries)
    for line in (ROOT / "checksums.sha256").read_text().splitlines():
        expected, relative = line.split("  ", 1)
        path = (ROOT / relative).resolve()
        if not path.is_relative_to(ROOT) or digest(path) != expected:
            raise ValueError(f"frozen corpus changed: {relative}")
    return manifest, queries


def build():
    (ROOT / "bin").mkdir(exist_ok=True)
    for name, package in (("videohelper", "./cmd/videohelper"), ("evalbridge", "./Eval/cmd/evalbridge")):
        subprocess.run(["go", "build", "-o", str(ROOT / "bin" / name), package], cwd=REPO, check=True)
    write_json(ROOT / "bin/build.json", {"code_revision": revision(), "binaries": {
        name: digest(ROOT / "bin" / name) for name in ("videohelper", "evalbridge")}})


def binary_provenance(name):
    try:
        sha256 = digest(ROOT / "bin" / name)
    except FileNotFoundError:
        return {"sha256": None, "build_revision": None}
    manifest = ROOT / "bin/build.json"
    built = read_json(manifest) if manifest.exists() else {}
    return {"sha256": sha256, "build_revision": built.get("code_revision")
            if built.get("binaries", {}).get(name) == sha256 else None}


def config_for(state):
    environment.runtime_env(state)  # Validates isolated resource names.
    path = Path(state["config_path"]).resolve()
    if not path.is_relative_to((ROOT / ".runtime").resolve()):
        raise ValueError("runtime config must be inside Eval/.runtime")
    config = yaml.safe_load(path.read_text())
    if (config["database"]["url_env"] != "VH_EVAL_DB_URL"
            or config["redis"]["addr_env"] != "VH_EVAL_REDIS_ADDR"
            or config["s3"]["bucket"] != state["bucket"]
            or config["s3"]["endpoint"] != state["s3_endpoint"]
            or config["ml"]["token_env"] != "VH_EVAL_ML_TOKEN"
            or config["ml"]["endpoints"] != [state["ml_endpoint"]]):
        raise ValueError("runtime config no longer matches isolated state")
    return config


def app_command(state, *args):
    config_for(state)
    return [str(ROOT / "bin/videohelper"), "-config", state["config_path"], *args]


def app_json(state, *args):
    result = subprocess.run(app_command(state, *args), cwd=REPO, env=environment.runtime_env(state),
                            text=True, capture_output=True, timeout=120, check=True)
    return json.loads(result.stdout) if result.stdout.strip() else None


def scope(snapshot):
    return {**snapshot["principal"], "video_ids": sorted(snapshot["mapping"].values())}


def inspect(state, request):
    config_for(state)
    result = subprocess.run([str(ROOT / "bin/evalbridge"), "-config", state["config_path"], "inspect"],
                            cwd=REPO, env=environment.runtime_env(state), input=json.dumps(request) + "\n",
                            text=True, capture_output=True, check=True, timeout=120)
    return json.loads(result.stdout)


def stop(process):
    if process.poll() is None:
        process.send_signal(signal.SIGINT)
        try:
            process.wait(timeout=30)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait()


def archive_objects(state, directory, inspection, manifest, mapping):
    client = environment.s3_client(state)
    inventory = []
    objects = directory / "objects"
    objects.mkdir()
    for page in client.get_paginator("list_objects_v2").paginate(Bucket=state["bucket"]):
        for item in page.get("Contents", []):
            key = item["Key"]
            path = (objects / key).resolve()
            if not path.is_relative_to(objects.resolve()):
                raise ValueError("unsafe object key in Eval bucket")
            path.parent.mkdir(parents=True, exist_ok=True)
            client.download_file(state["bucket"], key, str(path))
            inventory.append({"key": key, "sha256": digest(path), "bytes": path.stat().st_size})
    by_key = {item["key"]: item for item in inventory}
    by_id = {v["id"]: v for v in inspection["videos"]}
    for media in manifest["videos"]:
        record = by_id[mapping[media["id"]]]
        if by_key[record["source_key"]]["sha256"] != media["sha256"]:
            raise ValueError("uploaded source differs from frozen video")
        normalized = objects / by_key[record["normalized_video_key"]]["key"]
        info = json.loads(subprocess.check_output(["ffprobe", "-v", "error", "-show_streams", "-show_format",
                                                  "-of", "json", str(normalized)]))
        if abs(float(info["format"]["duration"]) * 1000 - media["duration_ms"]) > 100:
            raise ValueError("normalized video duration mismatch")
        if {s["codec_type"] for s in info["streams"]} != {"video", "audio"}:
            raise ValueError("normalized video lost audio or video")
    inventory.sort(key=lambda item: item["key"])
    write_json(directory / "objects.json", inventory)
    return inventory


def index_run(run_id, timeout_seconds):
    manifest, _ = corpus()
    output = fresh(ROOT / "runs", run_id + "-index")
    started = time.monotonic()
    summary = {"run_id": run_id, "started_at": datetime.now(UTC).isoformat(),
               "code_revision": revision(), "model_mode": "real", "status": "running",
               "build_provenance": {name: binary_provenance(name) for name in ("videohelper", "evalbridge")}}
    write_json(output / "index-run.json", summary)
    worker = None
    try:
        state = environment.prepare(run_id)
        write_json(output / "state.json", state)
        app_json(state, "migrate")
        principal = {key: int(value) for key, value in app_json(state, "local-init").items()}
        config = config_for(state)
        channels = yaml.safe_load(Path(config["channels_file"]).read_text())["channels"]
        for channel, settings in channels.items():
            for space in settings["active"] + settings["shadow"]:
                app_json(state, "space", "add", channel, space)
        mapping, uploads = {}, []
        for video in manifest["videos"]:
            before = time.monotonic()
            uploaded = app_json(state, "local-upload", str(ROOT / video["path"]))
            mapping[video["id"]] = int(uploaded["id"])
            uploads.append({"media_id": video["id"], "video_id": int(uploaded["id"]),
                            "elapsed_ms": round((time.monotonic() - before) * 1000, 2)})
            write_json(output / "mapping.json", {"principal": principal, "mapping": mapping, "uploads": uploads})
            print(f"uploaded {video['id']} -> {uploaded['id']}", flush=True)
        with (output / "worker.log").open("w") as log:
            worker = subprocess.Popen(app_command(state, "worker"), cwd=REPO, env=environment.runtime_env(state),
                                      stdout=log, stderr=log)
            deadline = time.monotonic() + timeout_seconds
            while True:
                statuses = [app_json(state, "local-video", str(video_id)) for video_id in mapping.values()]
                write_json(output / "stages.json", statuses)
                done = sum(item["video"]["status"] == "ready" for item in statuses)
                stages_done = sum(s["status"] == "done" for item in statuses for s in item["stages"])
                print(f"index progress: ready {done}/5, completed stages {stages_done}", flush=True)
                if any(item["video"]["status"] == "failed" for item in statuses):
                    raise RuntimeError("index pipeline failed; see stages.json and worker.log")
                if done == len(mapping):
                    break
                if worker.poll() is not None:
                    raise RuntimeError("Eval worker exited before indexing completed")
                if time.monotonic() >= deadline:
                    raise TimeoutError("indexing deadline exceeded; evidence retained")
                time.sleep(10)
            stop(worker)
        snapshot = {"schema_version": 1, "snapshot_id": run_id, "state": state, "principal": principal,
                    "mapping": mapping, "dataset_id": manifest["dataset_id"],
                    "dataset_sha256": digest(ROOT / "manifest.json"),
                    "labels_sha256": digest(ROOT / "annotations/queries.jsonl"),
                    "code_revision": summary["code_revision"], "model_mode": "real",
                    "build_provenance": summary["build_provenance"],
                    "models_config_sha256": digest(REPO / "ml/models.local.yaml"),
                    "model_fingerprints": {m["id"]: m["fingerprint"] for group in state["models"].values() for m in group}}
        inspection = inspect(state, scope(snapshot))
        write_json(output / "inspection.json", inspection)
        if not inspection["passed"]:
            raise RuntimeError("index acceptance failed; see inspection.json")
        directory = fresh(ROOT / "snapshots", run_id)
        write_json(directory / "inspection.json", inspection)
        snapshot["index_sha256"] = inspection["index_sha256"]
        snapshot["inspection_sha256"] = digest(directory / "inspection.json")
        with (directory / "database.dump").open("xb") as stream:
            subprocess.run(environment.pg_command("pg_dump", "-U", os.environ.get("VH_EVAL_DB_USER", "videohelper"),
                                                  "-d", state["db_name"], "-Fc", "--no-owner", "--no-acl"),
                           cwd=REPO, stdout=stream, check=True)
        snapshot["database_sha256"] = digest(directory / "database.dump")
        inventory = archive_objects(state, directory, inspection, manifest, mapping)
        snapshot["objects_sha256"] = digest(directory / "objects.json")
        write_json(directory / "config.yaml.json", config)
        snapshot["config_sha256"] = digest(directory / "config.yaml.json")
        write_json(directory / "snapshot.json", snapshot)
        summary.update(status="passed", elapsed_seconds=round(time.monotonic() - started, 2),
                       snapshot=str(directory.relative_to(ROOT)), index_sha256=snapshot["index_sha256"],
                       object_count=len(inventory), video_count=len(mapping), uploads=uploads)
        print(f"index acceptance passed; frozen snapshot: {directory}", flush=True)
    except (Exception, KeyboardInterrupt) as error:
        summary.update(status="cancelled" if isinstance(error, KeyboardInterrupt) else "failed",
                       error=type(error).__name__ + ": " + str(error),
                       elapsed_seconds=round(time.monotonic() - started, 2))
        raise
    finally:
        if worker is not None:
            stop(worker)
        write_json(output / "index-run.json", summary)


def load_snapshot(name):
    environment.names(name)
    directory = ROOT / "snapshots" / name
    snapshot = read_json(directory / "snapshot.json")
    if snapshot["snapshot_id"] != name:
        raise ValueError("snapshot id mismatch")
    for file, key in (("database.dump", "database_sha256"), ("objects.json", "objects_sha256"),
                      ("inspection.json", "inspection_sha256"), ("config.yaml.json", "config_sha256")):
        if digest(directory / file) != snapshot[key]:
            raise ValueError(f"snapshot file changed: {file}")
    if digest(ROOT / "manifest.json") != snapshot["dataset_sha256"] or digest(ROOT / "annotations/queries.jsonl") != snapshot["labels_sha256"]:
        raise ValueError("snapshot corpus/labels no longer match")
    if digest(REPO / "ml/models.local.yaml") != snapshot["models_config_sha256"]:
        raise ValueError("model manifest differs; create a new index baseline")
    return directory, snapshot


def verify_objects(directory):
    objects = (directory / "objects").resolve()
    for item in read_json(directory / "objects.json"):
        path = (objects / item["key"]).resolve()
        if not path.is_relative_to(objects) or digest(path) != item["sha256"]:
            raise ValueError("archived object changed or missing")


def restore_run(snapshot_id, run_id):
    directory, snapshot = load_snapshot(snapshot_id)
    verify_objects(directory)
    state = environment.prepare(run_id, indexing=False)
    with (directory / "database.dump").open("rb") as stream:
        subprocess.run(environment.pg_command("pg_restore", "-U", os.environ.get("VH_EVAL_DB_USER", "videohelper"),
                                              "-d", state["db_name"], "--no-owner", "--no-acl", "--exit-on-error"),
                       cwd=REPO, stdin=stream, check=True)
    client = environment.s3_client(state)
    for item in read_json(directory / "objects.json"):
        client.upload_file(str(directory / "objects" / item["key"]), state["bucket"], item["key"])
    result = inspect(state, scope(snapshot))
    if not result["passed"] or result["index_sha256"] != snapshot["index_sha256"]:
        raise ValueError("restored database does not match frozen index")
    write_json(Path(state["runtime_dir"]) / "restored-from.json", {
        "snapshot_id": snapshot_id, "index_sha256": result["index_sha256"], "status": "passed"})
    print(f"restore passed: {state['runtime_dir']}/state.json", flush=True)


def retrieve_run(snapshot_id, run_id, restored_state=None, overrides=None):
    from score import MODES, evaluate, write_report

    for flag, value in (overrides or {}).items():
        if value is None:
            continue
        if (flag not in ("k", "rrf-k", "merge-gap") or type(value) not in (int, float)
                or not math.isfinite(value) or value < 0
                or flag != "merge-gap" and value == 0
                or flag != "rrf-k" and type(value) is not int):
            raise ValueError(f"invalid search override: {flag}={value}")
    manifest, queries = corpus()
    _directory, snapshot = load_snapshot(snapshot_id)
    state = read_json(restored_state) if restored_state else snapshot["state"]
    config = config_for(state)
    model_info = environment.verify_models(state["ml_endpoint"], environment.runtime_env(state)["VH_EVAL_ML_TOKEN"])
    if model_info != snapshot["state"]["models"]:
        raise ValueError("live models differ from snapshot")
    output = fresh(ROOT / "runs", run_id)
    before = inspect(state, scope(snapshot))
    write_json(output / "inspection-before.json", before)
    if not before["passed"] or before["index_sha256"] != snapshot["index_sha256"]:
        raise ValueError("index differs from snapshot; retrieval refused")
    requests = [{**scope(snapshot), "query_id": q["id"], "text": q["query"], "mode": mode, "limit": 10}
                for mode in MODES for q in queries]
    request_file = output / "requests.jsonl"
    request_file.write_text("".join(json.dumps(q, ensure_ascii=False) + "\n" for q in requests))
    command = [str(ROOT / "bin/evalbridge"), "-config", state["config_path"]]
    search_config = dict(config["search"])
    for flag, value in (overrides or {}).items():
        if value is not None:
            command.extend(["-" + flag, str(value)])
            search_config[{"k": "k", "rrf-k": "rrf_k", "merge-gap": "merge_gap_cells"}[flag]] = value
    results = []
    ids = {str(video_id): media_id for media_id, video_id in snapshot["mapping"].items()}
    bridge_errors = []
    provenance = binary_provenance("evalbridge")
    with request_file.open() as source, (output / "search.log").open("w") as log, (output / "raw-responses.jsonl").open("wb") as raw:
        process = None
        try:
            process = subprocess.Popen([*command, "search"], cwd=REPO, env=environment.runtime_env(state),
                                       stdin=source, stdout=subprocess.PIPE, stderr=log)
            deadline = time.monotonic() + max(120, len(requests) * 120)
            with selectors.DefaultSelector() as selector:
                selector.register(process.stdout, selectors.EVENT_READ)
                while selector.get_map():
                    remaining = deadline - time.monotonic()
                    if remaining <= 0:
                        raise TimeoutError("retrieval deadline exceeded")
                    for key, _ in selector.select(timeout=min(1, remaining)):
                        chunk = os.read(key.fd, 65536)
                        if chunk:
                            raw.write(chunk)
                            raw.flush()
                        else:
                            selector.unregister(key.fileobj)
            process.wait(timeout=max(0, deadline - time.monotonic()))
        except (OSError, subprocess.SubprocessError, KeyboardInterrupt) as error:
            bridge_errors.append(type(error).__name__ + ": " + str(error))
        finally:
            if process is not None:
                stop(process)
                process.stdout.close()
        exit_code = process.returncode if process is not None else None
    expected = {(request["query_id"], request["mode"]) for request in requests}
    for number, line in enumerate((output / "raw-responses.jsonl").read_bytes().splitlines(), 1):
        try:
            result = json.loads(line)
            if (not isinstance(result, dict) or not isinstance(result.get("query_id"), str)
                    or not isinstance(result.get("mode"), str)
                    or (result["query_id"], result["mode"]) not in expected):
                raise ValueError("unknown result query/mode or invalid response object")
        except (ValueError, UnicodeDecodeError) as error:
            bridge_errors.append(f"response line {number}: {error}")
            continue
        if isinstance(result.get("moments"), list):
            for moment in result["moments"]:
                if not isinstance(moment, dict):
                    continue
                video_id = moment.get("video_id")
                moment["media_id"] = ids.get(str(video_id)) if type(video_id) in (int, str) else None
        results.append(result)
        print(f"retrieved {len(results)}/{len(requests)} {result['query_id']} {result['mode']}", flush=True)
    (output / "results.jsonl").write_text("".join(json.dumps(r, ensure_ascii=False) + "\n" for r in results))
    try:
        after = inspect(state, scope(snapshot))
    except (OSError, subprocess.SubprocessError, ValueError) as error:
        after = {"passed": False, "error": type(error).__name__ + ": " + str(error)}
    write_json(output / "inspection-after.json", after)
    stable = after["passed"] and before["index_sha256"] == after["index_sha256"]
    channels = yaml.safe_load(Path(config["channels_file"]).read_text())["channels"]
    metadata = {key: snapshot[key] for key in ("dataset_id", "dataset_sha256", "labels_sha256", "snapshot_id", "index_sha256", "model_fingerprints", "models_config_sha256")}
    metadata.update(run_id=run_id, code_revision=revision(), model_mode="real", search_config=search_config,
                    channels={"visual": ["visual"], "speech": ["speech"], "fused": sorted(channels)},
                    channel_config=channels, index_unchanged=stable, bridge_exit_code=exit_code,
                    bridge_errors=bridge_errors, bridge_build_provenance=provenance,
                    languages={q["id"]: "zh" if any("\u4e00" <= c <= "\u9fff" for c in q["query"]) else "en" for q in queries},
                    timing="sequential queries, model service preloaded; not a cold-start or load test",
                    media_languages={v["id"]: v["language"] for v in manifest["videos"]})
    write_json(output / "metadata.json", metadata)
    report = evaluate(queries, results, metadata)
    if not stable or exit_code or bridge_errors:
        report["passed"] = False
        report["limitations"] = [*report["limitations"], "Run invalid: index drift or bridge process/response failed; do not adopt as a baseline."]
    write_report(report, output)
    print(f"report: {output}/report.md; valid={report['passed']}", flush=True)
    return report["passed"]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    commands.add_parser("build")
    index = commands.add_parser("index")
    index.add_argument("--run-id", required=True)
    index.add_argument("--timeout-seconds", type=int, default=7200)
    retrieve = commands.add_parser("retrieve")
    retrieve.add_argument("--snapshot", required=True)
    retrieve.add_argument("--run-id", required=True)
    retrieve.add_argument("--state", help="state.json from a verified restore")
    retrieve.add_argument("--k", type=int)
    retrieve.add_argument("--rrf-k", type=float)
    retrieve.add_argument("--merge-gap", type=int)
    restore = commands.add_parser("restore")
    restore.add_argument("--snapshot", required=True)
    restore.add_argument("--run-id", required=True)
    args = parser.parse_args()
    if args.command == "build":
        build()
    elif args.command == "index":
        if args.timeout_seconds <= 0:
            parser.error("timeout must be positive")
        with operation_lock():
            index_run(args.run_id, args.timeout_seconds)
    elif args.command == "restore":
        with operation_lock():
            restore_run(args.snapshot, args.run_id)
    else:
        return 0 if retrieve_run(args.snapshot, args.run_id, args.state,
                                  {"k": args.k, "rrf-k": args.rrf_k, "merge-gap": args.merge_gap}) else 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
