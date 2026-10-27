"""Opt in: VH_TEST_REAL_CLAP=1
uv run --extra clap pytest tests/test_clap_real.py -q
Loads CLAP only; the existing WeMM/ASR service is not used or restarted.
"""

import io
import os
import wave

import numpy as np
import pytest
import yaml
from fastapi.testclient import TestClient
from test_clap import SPACE, model_config

from app.core.fingerprint import fingerprint
from app.main import create_app
from app.settings import Settings

pytestmark = pytest.mark.skipif(
    os.getenv("VH_TEST_REAL_CLAP") != "1",
    reason="Requires explicit CLAP-only real-model opt-in",
)


def test_real_clap_over_http_and_managed_files(tmp_path, monkeypatch):
    from huggingface_hub import constants

    monkeypatch.setattr(constants, "HF_HUB_OFFLINE", True)
    monkeypatch.setenv("HF_HUB_OFFLINE", "1")
    config = model_config()
    manifest = tmp_path / "models.yaml"
    manifest.write_text(yaml.safe_dump({"models": [config]}))
    audio = io.BytesIO()
    with wave.open(audio, "wb") as output:
        output.setnchannels(1)
        output.setsampwidth(2)
        output.setframerate(16000)
        samples = (np.sin(np.arange(32000) * 2 * np.pi * 440 / 16000) * 10000).astype("<i2")
        output.writeframes(samples.tobytes())
    (tmp_path / "tone.wav").write_bytes(audio.getvalue())
    app = create_app(
        Settings(
            token="clap-test",
            fake=False,
            models_file=manifest,
            local_media_root=tmp_path,
        )
    )
    with TestClient(
        app, headers={"X-Internal-Token": "clap-test", "X-Request-ID": "clap-real"}
    ) as client:
        spaces = client.get("/v1/spaces").json()["spaces"]
        assert len(spaces) == 1 and spaces[0]["id"] == SPACE
        expected = fingerprint(config, fake=False)
        assert spaces[0]["fingerprint"] == expected != fingerprint(config, fake=True)
        document = {
            "space": SPACE,
            "role": "document",
            "inputs": [
                {
                    "audio": {
                        "uri": "local://media/tone.wav",
                        "start_ms": 250,
                        "end_ms": 1750,
                    }
                }
            ],
        }
        outputs = []
        for request in (
            {"space": SPACE, "role": "query", "inputs": [{"text": "A sine wave tone"}]},
            document,
            document,
        ):
            response = client.post("/v1/embed", json=request)
            assert response.status_code == 200, response.text
            assert response.json()["fingerprint"] == expected
            vectors = np.asarray(response.json()["vectors"])
            assert vectors.shape == (1, 512) and np.isfinite(vectors).all()
            assert np.allclose(np.linalg.norm(vectors, axis=1), 1, atol=1e-4)
            outputs.append(vectors)
        assert np.allclose(outputs[1], outputs[2], atol=1e-7)
        assert client.post("/v1/embed", json={**document, "role": "query"}).status_code == 400
