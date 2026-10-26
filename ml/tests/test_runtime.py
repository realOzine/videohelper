from types import SimpleNamespace

import jsonschema
import yaml
from conftest import HEADERS
from fastapi.testclient import TestClient

from app.main import create_app
from app.runtime import describe_runtime
from app.settings import Settings


def test_runtime_is_authenticated_and_matches_loaded_snapshot(tmp_path, contract):
    source = Settings.models_file
    manifest = tmp_path / "models.yaml"
    manifest.write_bytes(source.read_bytes())
    settings = Settings(
        token="secret", fake=True, models_file=manifest, cache_dir=tmp_path / "cache"
    )
    with TestClient(create_app(settings, downloader=lambda *_: None)) as client:
        assert client.get("/v1/runtime", headers={"X-Request-ID": "x"}).status_code == 401
        headers = {**HEADERS, "X-Internal-Token": "secret"}
        response = client.get("/v1/runtime", headers=headers)
        assert response.status_code == 200
        body = response.json()
        assert body["runtime_version"] == 1
        assert body["fake"] is True
        configured = yaml.safe_load(manifest.read_text())["models"]
        assert [item["id"] for item in body["models"]] == [item["id"] for item in configured]
        assert all(item["adapter"] == "fake" for item in body["models"])
        assert all(item["device"] == "cpu" for item in body["models"])
        assert all(item["distribution"] is None for item in body["models"])
        fingerprints = client.get("/v1/spaces", headers=headers).json()
        assert {item["fingerprint"] for item in body["models"]} == {
            item["fingerprint"] for item in fingerprints["spaces"] + fingerprints["transcribers"]
        }
        manifest.write_text("models: []")
        assert client.get("/v1/runtime", headers=headers).json() == body
        schema = contract["paths"]["/v1/runtime"]["get"]["responses"]["200"]["content"][
            "application/json"
        ]["schema"]
        jsonschema.Draft202012Validator({**schema, "components": contract["components"]}).validate(
            body
        )
        assert "secret" not in response.text
        assert str(tmp_path) not in response.text


def test_real_runtime_reports_loaded_device_and_plugin_version(client, monkeypatch):
    registry = client.app.state.registry
    config = next(iter(registry.configs.values()))
    config = config.model_copy(update={"device": "mps", "dtype": "float16"})
    registry = SimpleNamespace(
        configs={**registry.configs, config.id: config}, spaces=registry.spaces
    )
    monkeypatch.setattr(
        "app.runtime.adapter_distribution",
        lambda name: {"name": f"videohelper-ml-{name}", "version": "0.1.0"},
    )
    runtime = describe_runtime(registry, fake=False)
    model = runtime.models[0]
    assert model.device == "mps" and model.dtype == "float16"
    assert model.adapter == config.adapter
    assert model.distribution == {"name": "videohelper-ml-wemm", "version": "0.1.0"}
    assert runtime.packages["numpy"]
