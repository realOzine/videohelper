"""Isolated local evaluation resources; never reuse or delete a database/bucket."""

import json
import os
import re
import subprocess
import sys
from pathlib import Path
from urllib.parse import quote, urlsplit
from urllib.request import HTTPRedirectHandler, Request, build_opener

import yaml

ROOT = Path(__file__).resolve().parents[2]
EVAL = ROOT / "Eval"
REDIS_ADDR = "127.0.0.1:56380"


def names(run_id):
    if not isinstance(run_id, str) or not re.fullmatch(
        r"[a-z0-9](?:[a-z0-9-]{0,38}[a-z0-9])?", run_id
    ):
        raise ValueError(
            "run_id must be 1-40 lowercase letters/digits/hyphens, ending alphanumeric"
        )
    return "vh_eval_" + run_id.replace("-", "_"), "vh-eval-" + run_id


def _defaults():
    return dict(
        line.split("=", 1)
        for line in (ROOT / ".env.example").read_text().splitlines()
        if line and not line.startswith("#") and "=" in line
    )


def runtime_env(state):
    """Credentials stay in the process environment, never in serialized state."""
    db_name, bucket = names(state["run_id"])
    if state["db_name"] != db_name or state["bucket"] != bucket:
        raise ValueError("state resource names do not match run_id")
    defaults = _defaults()
    env = os.environ.copy()
    # A caller's development DSN and AWS profile must not select evaluation resources.
    user = os.environ.get("VH_EVAL_DB_USER", "videohelper")
    password = os.environ.get("VH_EVAL_DB_PASSWORD", "videohelper-local")
    env["VH_EVAL_DB_URL"] = (
        f"postgres://{quote(user, safe='')}:{quote(password, safe='')}"
        f"@127.0.0.1:55432/{db_name}?sslmode=disable"
    )
    env["VH_EVAL_REDIS_ADDR"] = REDIS_ADDR
    env["VH_EVAL_ML_TOKEN"] = os.environ.get(
        "VH_EVAL_ML_TOKEN", defaults["VH_ML_TOKEN"]
    )
    for key in ("AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_REGION"):
        env[key] = os.environ.get("VH_EVAL_" + key, defaults[key])
    env["AWS_EC2_METADATA_DISABLED"] = "true"
    for key in (
        "AWS_SESSION_TOKEN",
        "AWS_PROFILE",
        "AWS_DEFAULT_PROFILE",
        "AWS_ENDPOINT_URL",
        "AWS_ENDPOINT_URL_S3",
    ):
        env.pop(key, None)
    return env


class _NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def expected_models():
    """Use the service's canonical config/fingerprint without constructing adapters."""
    sys.path.insert(0, str(ROOT / "ml"))
    from app.core.fingerprint import fingerprint
    from app.core.registry import ModelConfig

    expected = {"spaces": [], "transcribers": []}
    manifest = yaml.safe_load((ROOT / "ml/models.local.yaml").read_text())
    for raw in manifest["models"]:
        config = ModelConfig.model_validate(raw)
        value = {
            "id": config.id,
            "fingerprint": fingerprint(config.model_dump(), fake=False),
        }
        if config.kind == "embedder":
            for key in (
                "dim",
                "metric",
                "query_inputs",
                "doc_inputs",
                "max_span_ms",
                "max_batch",
            ):
                value[key] = getattr(config, key)
            expected["spaces"].append(value)
        else:
            for key in ("languages", "max_window_ms"):
                value[key] = getattr(config, key)
            expected["transcribers"].append(value)
    return expected


def verify_models(endpoint, token):
    parsed = urlsplit(endpoint)
    if (
        parsed.scheme not in ("http", "https")
        or not parsed.netloc
        or parsed.username
        or parsed.password
        or parsed.query
        or parsed.fragment
    ):
        raise ValueError("invalid ML endpoint")
    request = Request(
        endpoint.rstrip("/") + "/v1/spaces",
        headers={
            "X-Internal-Token": token,
            "X-Request-ID": "eval-discovery",
            "X-Timeout-Ms": "15000",
        },
    )
    try:
        with build_opener(_NoRedirect).open(request, timeout=15) as response:
            actual = json.load(response)
    except Exception as exc:
        raise RuntimeError("evaluation ML discovery failed") from exc
    expected = expected_models()
    if not isinstance(actual, dict) or set(actual) != set(expected):
        raise RuntimeError("ML discovery shape differs from models.local.yaml")
    for key in expected:
        values = actual[key]
        if not isinstance(values, list) or any(
            not isinstance(v, dict) or "id" not in v for v in values
        ):
            raise RuntimeError("invalid ML discovery entries")
        if sorted(values, key=lambda v: v["id"]) != sorted(
            expected[key], key=lambda v: v["id"]
        ):
            raise RuntimeError(
                "ML discovery differs from real models.local.yaml (fake/stale model or metadata)"
            )
    return actual


def pg_command(*args):
    """Container command prefix for pg_dump/pg_restore; no credentials in argv."""
    return [
        "docker",
        "compose",
        "-f",
        str(ROOT / "deploy/docker-compose.yml"),
        "exec",
        "-T",
        "postgres",
        *args,
    ]


def _psql(sql, *, database="postgres"):
    user = os.environ.get("VH_EVAL_DB_USER", "videohelper")
    return subprocess.run(
        pg_command(
            "psql",
            "-U",
            user,
            "-d",
            database,
            "-v",
            "ON_ERROR_STOP=1",
            "-At",
            "-c",
            sql,
        ),
        cwd=ROOT,
        check=True,
        text=True,
        capture_output=True,
    ).stdout.strip()


def start_redis():
    """Start only the small dedicated Eval service, and refuse leftover queue state."""
    compose = ["docker", "compose", "-f", str(EVAL / "deploy/redis-compose.yml")]
    subprocess.run([*compose, "up", "-d", "--wait"], cwd=ROOT, check=True)
    result = subprocess.run(
        [*compose, "exec", "-T", "redis", "redis-cli", "DBSIZE"],
        cwd=ROOT,
        check=True,
        capture_output=True,
        text=True,
    )
    if result.stdout.strip() != "0":
        raise RuntimeError(
            "Eval Redis is nonempty; stop its worker and explicitly clear Eval Redis before a new index"
        )


def s3_client(state):
    import boto3

    env = runtime_env(state)
    return boto3.client(
        "s3",
        endpoint_url=state["s3_endpoint"],
        aws_access_key_id=env["AWS_ACCESS_KEY_ID"],
        aws_secret_access_key=env["AWS_SECRET_ACCESS_KEY"],
        region_name=env["AWS_REGION"],
    )


def prepare(run_id, *, indexing=True):
    """Create a fresh database, bucket and config, or fail preserving all evidence."""
    db_name, bucket = names(run_id)
    runtime_dir = EVAL / ".runtime" / run_id
    if runtime_dir.exists():
        raise FileExistsError(f"evaluation runtime already exists: {runtime_dir}")
    config = yaml.safe_load((ROOT / "config/local.yaml").read_text())
    endpoint = os.environ.get("VH_EVAL_ML_ENDPOINT", config["ml"]["endpoints"][0])
    state = {
        "run_id": run_id,
        "db_name": db_name,
        "bucket": bucket,
        "config_path": str(runtime_dir / "config.yaml"),
        "runtime_dir": str(runtime_dir),
        "ml_endpoint": endpoint,
        "redis_addr": REDIS_ADDR,
        "s3_endpoint": config["s3"]["endpoint"],
    }
    env = runtime_env(state)
    state["models"] = verify_models(endpoint, env["VH_EVAL_ML_TOKEN"])
    if _psql(f"SELECT 1 FROM pg_database WHERE datname = '{db_name}'"):
        raise FileExistsError(f"evaluation database already exists: {db_name}")
    client = s3_client(state)
    if any(item["Name"] == bucket for item in client.list_buckets()["Buckets"]):
        raise FileExistsError(f"evaluation bucket already exists: {bucket}")
    if indexing:
        start_redis()
    runtime_dir.mkdir(parents=True, exist_ok=False)
    (runtime_dir / "tmp").mkdir()
    config["database"]["url_env"] = "VH_EVAL_DB_URL"
    config["redis"]["addr_env"] = "VH_EVAL_REDIS_ADDR"
    config["s3"]["bucket"] = bucket
    config["ml"]["endpoints"] = [endpoint]
    config["ml"]["token_env"] = "VH_EVAL_ML_TOKEN"
    config["media"]["tmp_dir"] = str(runtime_dir / "tmp")
    config["channels_file"] = str(ROOT / "config/channels.yaml")
    with Path(state["config_path"]).open("x") as stream:
        yaml.safe_dump(config, stream, sort_keys=False, allow_unicode=True)
    _psql(f'CREATE DATABASE "{db_name}"')
    client.create_bucket(Bucket=bucket)
    (runtime_dir / "state.json").write_text(
        json.dumps(state, ensure_ascii=False, indent=2) + "\n"
    )
    return state
