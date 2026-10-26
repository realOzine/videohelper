import tomllib
from pathlib import Path

import yaml

from app.core.fingerprint import fingerprint
from app.core.registry import Manifest

ML_ROOT = Path(__file__).parents[1]


def test_profiles_preserve_identities_and_isolate_cuda():
    local = Manifest.model_validate(yaml.safe_load((ML_ROOT / "models.local.yaml").read_text()))
    original = {model.id: model for model in local.models}
    profiles = {}
    for name in ("wemm-mps", "speech-audio-cpu", "wemm-cuda"):
        root = ML_ROOT / "profiles" / name
        profiles[name] = Manifest.model_validate(yaml.safe_load((root / "models.yaml").read_text()))
        project = tomllib.loads((root / "pyproject.toml").read_text())
        assert "workspace" not in project.get("tool", {}).get("uv", {})
        assert all("==" in dep for dep in project["project"]["dependencies"])
        assert all("editable" not in value for value in project["tool"]["uv"]["sources"].values())
        assert (root / "uv.lock").is_file()
    for name in ("wemm-mps", "speech-audio-cpu"):
        for model in profiles[name].models:
            assert model == original[model.id]
    cuda = profiles["wemm-cuda"].models[0]
    mps = profiles["wemm-mps"].models[0]
    assert cuda.id != mps.id
    assert cuda.device == "cuda"
    assert cuda.preprocess["adapter_version"] != mps.preprocess["adapter_version"]
    assert fingerprint(cuda.model_dump(), fake=False) != fingerprint(mps.model_dump(), fake=False)
