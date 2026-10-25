import pytest
import yaml

from app.core.fingerprint import fingerprint
from app.core.registry import Registry
from app.settings import Settings


def test_fingerprint_is_stable_and_fake_is_distinct():
    config = {
        "adapter": "wemm",
        "weights": "a",
        "revision": "r1",
        "dtype": "float32",
        "preprocess": {"a": 1, "b": 2},
    }
    first = fingerprint(config, fake=False)
    assert fingerprint({**config, "preprocess": {"b": 2, "a": 1}}, fake=False) == first
    assert fingerprint({**config, "revision": "r2"}, fake=False) != first
    assert fingerprint(config, fake=True) != first


def test_fake_must_be_explicit_and_settings_require_token(tmp_path):
    with pytest.raises(ValueError, match="ML_FAKE=1"):
        Registry(Settings.models_file, fake=False)
    with pytest.raises(ValueError, match="ML_INTERNAL_TOKEN"):
        Settings(token="")


@pytest.mark.parametrize("mutation", ["duplicate", "revision", "concurrency"])
def test_invalid_registry(tmp_path, mutation):
    manifest = yaml.safe_load(Settings.models_file.read_text())
    if mutation == "duplicate":
        manifest["models"].append(manifest["models"][0])
    elif mutation == "revision":
        manifest["models"][0].pop("revision")
    else:
        manifest["models"][0]["max_concurrency"] = 0
    path = tmp_path / "models.yaml"
    path.write_text(yaml.safe_dump(manifest))
    with pytest.raises(ValueError):
        Registry(path, fake=True)
