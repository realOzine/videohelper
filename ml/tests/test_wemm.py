import numpy as np
import pytest
import yaml

from app.adapters.base import ImageInput, MediaInput, TextInput
from app.adapters.wemm import WeMMEmbedder
from app.api.errors import MLError
from app.api.schemas import SpaceInfo
from app.core.fingerprint import fingerprint
from app.core.registry import Registry
from app.settings import Settings


def model_config():
    path = Settings.models_file.with_name("models.wemm.yaml")
    return yaml.safe_load(path.read_text())["models"][0]


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
    instance = WeMMEmbedder(info, config)
    instance.device = "cpu"
    return instance


def test_real_registry_uses_one_gate_and_distinct_output_fingerprint(monkeypatch):
    monkeypatch.setattr(WeMMEmbedder, "load", lambda self: None)
    registry = Registry(Settings.models_file.with_name("models.wemm.yaml"), fake=False)
    assert registry.gates["wemm-2b@r1", "query"] is registry.gates["wemm-2b@r1", "document"]
    config = model_config()
    baseline = fingerprint(config, fake=False)
    assert baseline != fingerprint(config, fake=True)
    assert baseline != fingerprint({**config, "dim": 2048}, fake=False)
    changed = {**config, "preprocess": {**config["preprocess"], "frames_per_clip": 8}}
    assert baseline != fingerprint(changed, fake=False)


def test_unreviewed_revision_and_unsafe_parallelism_are_rejected():
    instance = adapter()
    for change in ({"revision": "main"}, {"max_concurrency": 2}):
        with pytest.raises(ValueError):
            WeMMEmbedder(instance.info, {**instance.config, **change})


class Inputs(dict):
    def to(self, device):
        return self


class Processor:
    def apply_chat_template(self, messages, **kwargs):
        self.messages = messages
        return "prompt"

    def __call__(self, **kwargs):
        self.kwargs = kwargs
        return Inputs(input_ids=np.zeros((1, 10), dtype=np.int64))


def test_image_video_preprocessing_uses_exact_window_without_sampling_twice(media_file, tmp_path):
    from PIL import Image

    instance = adapter()
    instance.processor = Processor()
    image = tmp_path / "image.png"
    Image.new("RGB", (12, 8), "red").save(image)
    instance._inputs(ImageInput(image))
    assert instance.processor.kwargs["images"][0].size == (12, 8)
    instance._inputs(MediaInput("video", media_file, 500, 1500))
    kwargs = instance.processor.kwargs
    assert kwargs["videos"][0].shape == (4, 32, 32, 3)
    metadata = kwargs["videos_kwargs"]["video_metadata"][0]
    assert metadata["fps"] == 4
    assert metadata["frames_indices"] == [0, 1, 2, 3]
    assert kwargs["videos_kwargs"]["do_sample_frames"] is False
    with pytest.raises(MLError):
        instance._inputs(MediaInput("audio", media_file, 0, 1000))


def test_embed_truncates_and_renormalizes_without_cache(monkeypatch):
    torch = pytest.importorskip("torch")
    instance = adapter()
    calls = []

    class Model:
        def embedding(self, **kwargs):
            calls.append(kwargs)
            return torch.arange(1, 2049, dtype=torch.float32).reshape(1, -1)

    instance.model = Model()
    monkeypatch.setattr(instance, "_inputs", lambda item: {"input_ids": item.text})
    result = instance.embed([TextInput("one"), TextInput("two")], "document")
    assert result.shape == (2, 1024)
    assert result.dtype == np.float32
    assert np.allclose(np.linalg.norm(result, axis=1), 1)
    assert [call["input_ids"] for call in calls] == ["one", "two"]
    assert all(call["use_cache"] is False for call in calls)
