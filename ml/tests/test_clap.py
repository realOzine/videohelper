from types import SimpleNamespace

import numpy as np
import pytest
import yaml

from app.adapters.base import MediaInput, TextInput
from app.adapters.clap import CLAPEmbedder
from app.api.errors import MLError
from app.api.schemas import SpaceInfo
from app.core.fingerprint import fingerprint
from app.core.registry import ModelConfig, Registry
from app.settings import Settings

SPACE = "clap-htsat-unfused@r1"


def model_config():
    manifest = yaml.safe_load(Settings.models_file.with_name("models.local.yaml").read_text())
    return ModelConfig.model_validate(
        next(item for item in manifest["models"] if item["id"] == SPACE)
    ).model_dump()


def adapter():
    config = model_config()
    info = SpaceInfo(
        **{
            key: config[key]
            for key in (
                "id",
                "dim",
                "metric",
                "query_inputs",
                "doc_inputs",
                "max_span_ms",
                "max_batch",
            )
        },
        fingerprint=fingerprint(config, fake=False),
    )
    return CLAPEmbedder(info, config)


def test_clap_registration_pins_and_fingerprints(tmp_path, monkeypatch):
    config = model_config()
    monkeypatch.setattr(CLAPEmbedder, "load", lambda self: None)
    path = tmp_path / "models.yaml"
    path.write_text(yaml.safe_dump({"models": [config]}))
    registry = Registry(path, fake=False)
    assert isinstance(registry.embedders[SPACE], CLAPEmbedder)
    assert registry.gates[SPACE, "query"] is not registry.gates[SPACE, "document"]
    assert registry.spaces().spaces[0].fingerprint != fingerprint(config, fake=True)
    info = registry.spaces().spaces[0]
    for change in ({"revision": "main"}, {"dtype": "float16"}):
        with pytest.raises(ValueError):
            CLAPEmbedder(info, {**config, **change})
    with pytest.raises(ValueError):
        CLAPEmbedder(info.model_copy(update={"dim": 1024}), config)


def test_bounded_audio_never_reaches_random_crop(monkeypatch, tmp_path):
    import videohelper_ml_clap as clap

    instance = adapter()
    monkeypatch.setattr(clap, "load", lambda *args: np.zeros(480001, np.float32))
    with pytest.raises(MLError):
        instance._audio_inputs(MediaInput("audio", tmp_path / "unused", 0, 10000))
    with pytest.raises(MLError):
        instance._audio_inputs(MediaInput("audio", tmp_path / "unused", 0, 10001))


def test_short_audio_padding_is_deterministic(monkeypatch, tmp_path):
    pytest.importorskip("torch")
    import videohelper_ml_clap as clap
    from transformers import ClapFeatureExtractor

    instance = adapter()
    instance.processor = SimpleNamespace(feature_extractor=ClapFeatureExtractor())
    samples = np.sin(np.arange(12000) * 0.1).astype(np.float32)
    monkeypatch.setattr(clap, "load", lambda *args: samples)
    monkeypatch.setattr(
        np.random, "randint", lambda *args: pytest.fail("random cropping was invoked")
    )
    item = MediaInput("audio", tmp_path / "unused", 1000, 1250)
    first, second = instance._audio_inputs(item), instance._audio_inputs(item)
    assert np.array_equal(first["input_features"], second["input_features"])
    assert first["input_features"].shape == (1, 1, 1001, 64)
    assert not first["is_longer"].any()


def test_projection_output_api_and_unchanged_text(monkeypatch, tmp_path):
    torch = pytest.importorskip("torch")
    instance = adapter()
    texts, methods = [], []

    def tokenize(text, **kwargs):
        texts.append(text)
        return {"input_ids": torch.ones((1, 3), dtype=torch.long)}

    class Model:
        def get_text_features(self, **kwargs):
            methods.append("text")
            return SimpleNamespace(pooler_output=torch.ones((1, 512)))

        def get_audio_features(self, **kwargs):
            methods.append("audio")
            return SimpleNamespace(pooler_output=torch.ones((1, 512)))

    instance.processor = SimpleNamespace(tokenizer=tokenize)
    instance.model = Model()
    monkeypatch.setattr(instance, "_audio_inputs", lambda item: {})
    for item, role in [
        (TextInput("狗叫声 unchanged"), "query"),
        (MediaInput("audio", tmp_path / "x", 0, 1000), "document"),
    ]:
        result = instance.embed([item], role)
        assert result.shape == (1, 512)
        assert np.isfinite(result).all() and np.allclose(np.linalg.norm(result, axis=1), 1)
    assert texts == ["狗叫声 unchanged"]
    assert methods == ["text", "audio"]
    instance.processor.tokenizer = lambda *args, **kwargs: {
        "input_ids": torch.ones((1, 513), dtype=torch.long),
    }
    with pytest.raises(MLError):
        instance.embed([TextInput("long query")], "query")


def test_fake_audio_http_contract(client):
    response = client.get("/v1/spaces")
    info = next(space for space in response.json()["spaces"] if space["id"] == SPACE)
    assert info["dim"] == 512 and info["doc_inputs"] == ["audio"]
    request = {
        "space": SPACE,
        "role": "document",
        "inputs": [
            {
                "audio": {
                    "uri": "s3://test/audio.mp4",
                    "start_ms": 100,
                    "end_ms": 1900,
                }
            }
        ],
    }
    response = client.post("/v1/embed", json=request)
    assert response.status_code == 200, response.text
    assert response.json()["fingerprint"] == info["fingerprint"]
    assert np.asarray(response.json()["vectors"]).shape == (1, 512)
    request["role"] = "query"
    assert client.post("/v1/embed", json=request).status_code == 400
    request["role"] = "document"
    request["inputs"][0]["audio"]["end_ms"] = 10101
    assert client.post("/v1/embed", json=request).status_code == 400
