"""Opt in: VH_TEST_REAL_MODELS=1 VH_TEST_SPEECH_FILE=/absolute/speech.mp4
uv run --extra wemm --extra asr --extra clap pytest tests/test_real_models.py -q
Uses cached pinned weights, a temporary loopback service, and managed temporary media.
VH_TEST_SPEECH_FILE may point to the authorized synthetic TEXT clip with 3s leading silence.
"""

import os
import shutil
import socket
import subprocess
import sys
import time
from pathlib import Path

import av
import httpx
import numpy as np
import pytest
import yaml
from PIL import Image

from app.core.fingerprint import fingerprint
from app.core.registry import ModelConfig

pytestmark = pytest.mark.skipif(
    os.getenv("VH_TEST_REAL_MODELS") != "1",
    reason="Requires explicit real-model opt-in",
)
ML_ROOT = Path(__file__).parents[1]
TEXT = "欢迎使用视频助手。今天我们学习如何搜索视频内容。"


@pytest.fixture
def real_service(tmp_path):
    speech = Path(os.getenv("VH_TEST_SPEECH_FILE", ""))
    if not speech.is_file():
        pytest.skip("VH_TEST_SPEECH_FILE must contain TEXT after 3s leading silence")
    image = Image.new("RGB", (320, 240), "red")
    image_path, video_path = tmp_path / "image.png", tmp_path / "video.mp4"
    image.save(image_path)
    with av.open(str(video_path), "w") as container:
        stream = container.add_stream("mpeg4", rate=4)
        stream.width, stream.height, stream.pix_fmt = 320, 240, "yuv420p"
        for index in range(8):
            frame = av.VideoFrame.from_image(image)
            frame.pts = index
            for packet in stream.encode(frame):
                container.mux(packet)
        for packet in stream.encode():
            container.mux(packet)
    environment = {
        **os.environ,
        "HF_HUB_OFFLINE": "1",
        "HF_HUB_DISABLE_PROGRESS_BARS": "1",
        "ML_FAKE": "0",
        "ML_INTERNAL_TOKEN": "real-ml-test",
        "ML_MODELS_FILE": str(ML_ROOT / "models.local.yaml"),
        "ML_LOCAL_MEDIA_ROOT": str(tmp_path),
    }
    shutil.copyfile(speech, tmp_path / "speech.mp4")
    process = None
    try:
        with socket.socket() as listener:
            listener.bind(("127.0.0.1", 0))
            port = listener.getsockname()[1]
        log = tmp_path / "uvicorn.log"
        with log.open("w") as output:
            process = subprocess.Popen(
                [
                    sys.executable,
                    "-m",
                    "uvicorn",
                    "app.main:app",
                    "--host",
                    "127.0.0.1",
                    "--port",
                    str(port),
                    "--log-level",
                    "error",
                ],
                cwd=ML_ROOT,
                env=environment,
                stdout=output,
                stderr=subprocess.STDOUT,
            )
        with httpx.Client(
            base_url=f"http://127.0.0.1:{port}",
            timeout=60,
            headers={
                "X-Internal-Token": "real-ml-test",
                "X-Request-ID": "real-local-test",
                "X-Timeout-Ms": "60000",
            },
        ) as client:
            deadline = time.monotonic() + 120
            while True:
                assert process.poll() is None, log.read_text()[-4000:]
                try:
                    if client.get("/readyz").status_code == 200:
                        break
                except httpx.ConnectError:
                    pass
                assert time.monotonic() < deadline, log.read_text()[-4000:]
                time.sleep(0.1)
            yield client
    finally:
        if process is not None:
            process.terminate()
            try:
                process.wait(timeout=15)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait(timeout=5)


def test_real_models_over_http_and_managed_files(real_service):
    client = real_service
    manifest = yaml.safe_load((ML_ROOT / "models.local.yaml").read_text())
    configs = {
        item["id"]: ModelConfig.model_validate(item).model_dump() for item in manifest["models"]
    }
    response = client.get("/v1/spaces")
    assert response.status_code == 200, response.text
    for info in response.json()["spaces"] + response.json()["transcribers"]:
        assert info["fingerprint"] == fingerprint(configs[info["id"]], fake=False)
        assert info["fingerprint"] != fingerprint(configs[info["id"]], fake=True)
    for role, item in [
        ("query", {"text": "红色画面"}),
        ("document", {"image": {"uri": "local://media/image.png"}}),
        ("document", {"video": {"uri": "local://media/video.mp4", "start_ms": 0, "end_ms": 2000}}),
    ]:
        response = client.post(
            "/v1/embed", json={"space": "wemm-2b@r1", "role": role, "inputs": [item]}
        )
        assert response.status_code == 200, response.text
        body = response.json()
        assert body["fingerprint"] == fingerprint(configs[body["space"]], fake=False)
        vectors = np.asarray(body["vectors"])
        assert vectors.shape == (1, body["dim"]) == (1, 1024)
        assert np.isfinite(vectors).all()
        assert np.allclose(np.linalg.norm(vectors, axis=1), 1, atol=1e-4)
        assert response.headers["X-Request-ID"] == "real-local-test"
    response = client.post(
        "/v1/embed",
        json={
            "space": "wemm-2b@r1",
            "role": "query",
            "inputs": [{"image": {"uri": "local://media/image.png"}}],
        },
    )
    assert response.status_code == 400
    assert response.json()["error"]["code"] == "invalid_input"
    body = {
        "model": "paraformer-zh@r1",
        "uri": "local://media/speech.mp4",
        "language": "zh",
        "start_ms": 2500,
        "end_ms": 9000,
    }
    response = client.post("/v1/transcribe", json=body)
    assert response.status_code == 200, response.text
    transcript = response.json()
    assert transcript["fingerprint"] == fingerprint(configs[body["model"]], fake=False)
    assert "".join(sentence["text"] for sentence in transcript["sentences"]) == TEXT
    sentence_start = word_start = 2500
    for sentence in transcript["sentences"]:
        assert sentence_start <= sentence["start_ms"] < sentence["end_ms"] <= 9000
        sentence_start = sentence["start_ms"]
        for word in sentence["words"]:
            assert (
                max(word_start, sentence["start_ms"]) <= word["s"] < word["e"] <= sentence["end_ms"]
            )
            word_start = word["s"]
    silence = client.post("/v1/transcribe", json={**body, "start_ms": 0, "end_ms": 2000})
    assert silence.status_code == 200, silence.text
    assert silence.json()["sentences"] == []
