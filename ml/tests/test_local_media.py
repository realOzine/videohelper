import jsonschema
import pytest
from conftest import HEADERS
from fastapi.testclient import TestClient
from pydantic import ValidationError

from app.api.errors import MLError
from app.api.schemas import ImageRef
from app.main import create_app
from app.media.local import local_media_path
from app.settings import Settings


def test_settings_require_absolute_managed_root(tmp_path, monkeypatch):
    from pathlib import Path

    for root in (None, Path("relative")):
        with pytest.raises(ValueError, match="ML_LOCAL_MEDIA_ROOT"):
            Settings(token="private-token", local_media_root=root)
    monkeypatch.setenv("ML_INTERNAL_TOKEN", "private-token")
    monkeypatch.delenv("ML_LOCAL_MEDIA_ROOT", raising=False)
    with pytest.raises(ValueError, match="ML_LOCAL_MEDIA_ROOT"):
        Settings.from_env()
    monkeypatch.setenv("ML_LOCAL_MEDIA_ROOT", str(tmp_path))
    assert Settings.from_env().local_media_root == tmp_path


@pytest.mark.parametrize(
    "key",
    ["../outside", "/etc/passwd", "a/../b", "a/%2e%2e/b", "a\\b", "a?x", "a#x", "a\n", ".", "a//b"],
)
def test_invalid_local_key_rejected_by_schema_and_resolver(tmp_path, key, contract):
    uri = "local://media/" + key
    with pytest.raises(ValidationError):
        ImageRef(uri=uri)
    with pytest.raises(MLError):
        local_media_path(uri, tmp_path)
    schema = {"$ref": "#/components/schemas/ImageRef", "components": contract["components"]}
    with pytest.raises(jsonschema.ValidationError):
        jsonschema.Draft202012Validator(schema).validate({"uri": uri})


def test_local_media_is_opt_in_regular_and_never_downloaded(tmp_path):
    root = tmp_path / "media"
    root.mkdir()
    path = root / "source.mp4"
    path.write_bytes(b"original")
    uri = "local://media/source.mp4"
    with pytest.raises(MLError):
        local_media_path(uri, None)
    assert local_media_path(uri, root) == path
    assert path.read_bytes() == b"original"
    assert list(root.iterdir()) == [path]
    for name in ("empty", "directory", "missing"):
        if name == "empty":
            (root / name).touch()
        elif name == "directory":
            (root / name).mkdir()
        with pytest.raises(MLError):
            local_media_path("local://media/" + name, root)


def test_symlink_file_and_parent_rejected(tmp_path):
    root = tmp_path / "media"
    root.mkdir()
    outside = tmp_path / "secret"
    outside.write_bytes(b"secret")
    try:
        (root / "link").symlink_to(outside)
        (root / "parent").symlink_to(tmp_path, target_is_directory=True)
    except OSError:
        pytest.skip("symlink creation is unavailable")
    for key in ("link", "parent/secret"):
        with pytest.raises(MLError):
            local_media_path("local://media/" + key, root)


def test_local_media_runs_through_authenticated_ml_routes(tmp_path, media_file):
    settings = Settings(
        token="test-secret",
        fake=True,
        local_media_root=media_file.parent,
    )
    uri = "local://media/" + media_file.name
    with TestClient(create_app(settings), headers=HEADERS) as client:
        response = client.post(
            "/v1/embed",
            json={
                "space": "wemm-2b@r1",
                "role": "document",
                "inputs": [{"video": {"uri": uri, "start_ms": 0, "end_ms": 1000}}],
            },
        )
        assert response.status_code == 200, response.text

        response = client.post(
            "/v1/transcribe",
            json={
                "model": "paraformer-zh@r1",
                "uri": uri,
                "language": "zh",
                "start_ms": 0,
                "end_ms": 1000,
            },
        )
        assert response.status_code == 200, response.text


@pytest.mark.parametrize(
    "uri", ["s3://bucket/key", "https://example.org/video", "file:///etc/passwd"]
)
def test_external_media_protocols_are_rejected(uri, tmp_path, contract):
    with pytest.raises(ValidationError):
        ImageRef(uri=uri)
    with pytest.raises(MLError):
        local_media_path(uri, tmp_path)
    schema = {"$ref": "#/components/schemas/ImageRef", "components": contract["components"]}
    with pytest.raises(jsonschema.ValidationError):
        jsonschema.Draft202012Validator(schema).validate({"uri": uri})
