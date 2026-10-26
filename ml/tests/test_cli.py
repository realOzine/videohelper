import json
from pathlib import Path
from types import SimpleNamespace

from app.cli import main

MODELS = Path(__file__).parents[1] / "profiles/speech-audio-cpu/models.yaml"


def test_prepare_dispatches_each_selected_plugin_without_serving(monkeypatch, capsys):
    selected, prepared = [], []

    def load(name, kind):
        selected.append((name, kind))
        return SimpleNamespace(prepare=prepared.append)

    monkeypatch.setattr("app.cli.load_adapter", load)
    monkeypatch.setattr("sys.argv", ["videohelper-ml", "prepare", "--models", str(MODELS)])
    main()
    assert selected == [("funasr", "transcriber"), ("clap", "embedder")]
    assert [config["id"] for config in prepared] == ["paraformer-zh@r1", "clap-htsat-unfused@r1"]
    assert all(len(config["revision"]) == 40 for config in prepared)
    assert "prepared" in capsys.readouterr().out


def test_describe_does_not_load_model_plugins(monkeypatch, capsys):
    def unexpected(*_):
        raise AssertionError("describe must not import a model plugin")

    monkeypatch.setattr("app.cli.load_adapter", unexpected)
    monkeypatch.setattr("sys.argv", ["videohelper-ml", "describe", "--models", str(MODELS)])
    main()
    described = json.loads(capsys.readouterr().out)
    assert [model["adapter"] for model in described["models"]] == ["funasr", "clap"]
