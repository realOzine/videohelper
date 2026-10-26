from types import SimpleNamespace

import pytest
import yaml

from app.core.plugins import load_adapter
from app.core.registry import Registry
from app.settings import Settings


def test_only_configured_entry_point_is_loaded(monkeypatch, tmp_path):
    class Embedder:
        kind = "embedder"
        shared_query_gate = True

        def __init__(self, info, config):
            self.info = info

        def load(self):
            pass

        @staticmethod
        def prepare(config):
            pass

        def embed(self, inputs, role):
            pass

    requested = []

    def entries(*, group, name):
        assert group == "videohelper.ml.adapters.v1"
        requested.append(name)
        assert name == "external"
        return [SimpleNamespace(load=lambda: Embedder)]

    monkeypatch.setattr("app.core.plugins.entry_points", entries)
    config = yaml.safe_load(Settings.models_file.read_text())["models"][0]
    config.update(adapter="external", id="external@r1")
    path = tmp_path / "models.yaml"
    path.write_text(yaml.safe_dump({"models": [config]}))
    registry = Registry(path, fake=False)
    assert requested == ["external"]
    assert registry.gates[config["id"], "query"] is registry.gates[config["id"], "document"]
    assert registry.configs[config["id"]].adapter == "external"


@pytest.mark.parametrize("fault", ["missing", "duplicate", "wrong_kind", "invalid", "import"])
def test_missing_duplicate_and_incompatible_plugins_fail_clearly(monkeypatch, fault):
    def load():
        if fault == "import":
            raise ImportError("missing inference dependency")
        if fault == "invalid":
            return object()
        return type(
            "Adapter",
            (),
            {
                "kind": "transcriber" if fault == "wrong_kind" else "embedder",
                "load": lambda self: None,
                "prepare": lambda config: None,
                "embed": lambda self, inputs, role: None,
                "shared_query_gate": False,
            },
        )

    entry = SimpleNamespace(load=load)
    entries = [] if fault == "missing" else [entry, entry] if fault == "duplicate" else [entry]
    monkeypatch.setattr("app.core.plugins.entry_points", lambda **kwargs: entries)
    match = {"missing": "not installed", "duplicate": "Duplicate", "import": "Cannot import"}
    with pytest.raises(ValueError, match=match.get(fault, "Incompatible")) as error:
        load_adapter("external", "embedder")
    if fault == "missing":
        assert "Install its plugin package" in str(error.value)


def test_explicit_fake_never_discovers_plugins(monkeypatch):
    monkeypatch.setattr(
        "app.core.plugins.entry_points", lambda **kwargs: pytest.fail("fake discovered a plugin")
    )
    assert len(Registry(Settings.models_file, fake=True).embedders) == 2


def test_prepare_validates_cached_weights_without_loading_models(monkeypatch, tmp_path):
    import hashlib
    import sys

    import videohelper_ml_clap as clap
    import videohelper_ml_funasr as funasr
    import videohelper_ml_wemm as wemm

    downloads = []

    def snapshot(**kwargs):
        downloads.append(kwargs)
        return str(tmp_path)

    monkeypatch.setitem(
        sys.modules,
        "huggingface_hub",
        SimpleNamespace(constants=SimpleNamespace(HF_HUB_OFFLINE=True), snapshot_download=snapshot),
    )
    configs = yaml.safe_load(Settings.models_file.with_name("models.local.yaml").read_text())[
        "models"
    ]
    for name in (
        "config.json",
        "processor_config.json",
        "tokenizer.json",
        "tokenizer_config.json",
        "chat_template.jinja",
        "model.safetensors",
        "preprocessor_config.json",
        "pytorch_model.bin",
        "special_tokens_map.json",
        "merges.txt",
        "vocab.json",
    ):
        (tmp_path / name).touch()
    code = tmp_path / "modeling_wemm_embedding.py"
    code.write_text("reviewed test source")
    monkeypatch.setattr(wemm, "REMOTE_CODE_SHA256", hashlib.sha256(code.read_bytes()).hexdigest())
    assert wemm.WeMMEmbedder.prepare(configs[0]) == tmp_path
    code.write_text("tampered source")
    with pytest.raises(RuntimeError, match="reviewed source"):
        wemm.WeMMEmbedder.prepare(configs[0])
    for name in (
        "model.pt",
        "config.yaml",
        "configuration.json",
        "am.mvn",
        "tokens.json",
        "seg_dict",
    ):
        (tmp_path / name).touch()
    assert funasr.FunASRTranscriber.prepare(configs[1]) == [str(tmp_path)] * 3
    # CLAP passes the repository as a positional argument, unlike the other adapters.
    monkeypatch.setattr(
        sys.modules["huggingface_hub"], "snapshot_download", lambda *a, **kw: str(tmp_path)
    )
    assert clap.CLAPEmbedder.prepare(configs[2]) == tmp_path
    assert all(call["local_files_only"] is True for call in downloads)


@pytest.mark.parametrize("adapter", ["wemm", "clap"])
def test_prepare_rejects_incomplete_offline_snapshot(monkeypatch, tmp_path, adapter):
    import sys

    monkeypatch.setitem(
        sys.modules,
        "huggingface_hub",
        SimpleNamespace(
            constants=SimpleNamespace(HF_HUB_OFFLINE=True),
            snapshot_download=lambda *args, **kwargs: str(tmp_path),
        ),
    )
    configs = yaml.safe_load(Settings.models_file.with_name("models.local.yaml").read_text())[
        "models"
    ]
    config = next(config for config in configs if config["adapter"] == adapter)
    with pytest.raises(RuntimeError, match="snapshot is incomplete"):
        load_adapter(adapter, "embedder").prepare(config)
