"""Build and install real wheels outside the checkout without model downloads."""

import os
import shutil
import subprocess
import sys
from pathlib import Path

import pytest

ML_ROOT = Path(__file__).parents[1]


def run(*args, cwd, **kwargs):
    result = subprocess.run(args, cwd=cwd, text=True, capture_output=True, **kwargs)
    assert result.returncode == 0, result.stdout + result.stderr
    return result.stdout


@pytest.fixture(scope="module")
def wheels(tmp_path_factory):
    if not shutil.which("uv"):
        pytest.skip("wheel installation check requires uv")
    directory = tmp_path_factory.mktemp("wheels")
    for source in [ML_ROOT, *(ML_ROOT / "plugins" / name for name in ("wemm", "funasr", "clap"))]:
        run(
            "uv",
            "build",
            "--offline",
            "--wheel",
            "--out-dir",
            str(directory),
            str(source),
            cwd=directory,
        )
    return directory


@pytest.fixture
def installed(wheels, tmp_path):
    environment = tmp_path / "venv"
    run(
        "uv",
        "sync",
        "--project",
        str(ML_ROOT),
        "--locked",
        "--offline",
        "--no-dev",
        "--no-install-project",
        "--python",
        sys.executable,
        cwd=tmp_path,
        env={**os.environ, "UV_PROJECT_ENVIRONMENT": str(environment)},
    )
    python = environment / ("Scripts/python.exe" if os.name == "nt" else "bin/python")
    wheel = wheels / "videohelper_ml-0.1.0-py3-none-any.whl"
    run(
        "uv",
        "pip",
        "install",
        "--offline",
        "--no-deps",
        "--python",
        str(python),
        str(wheel),
        cwd=tmp_path,
    )
    return python, tmp_path


def test_packaged_fake_manifest_matches_source():
    assert (ML_ROOT / "app/resources/models.yaml").read_bytes() == (
        ML_ROOT / "models.yaml"
    ).read_bytes()


def test_core_wheel_runs_fake_without_plugins_or_heavy_dependencies(installed):
    python, directory = installed
    run(
        str(python),
        "-I",
        "-c",
        """
import importlib.util
from importlib.metadata import requires
from app.core.registry import Registry
from app.settings import Settings
for module in ('torch', 'transformers', 'funasr', 'videohelper_ml_wemm'):
    assert importlib.util.find_spec(module) is None, module
assert all('torch' not in requirement for requirement in requires('videohelper-ml'))
registry = Registry(Settings.models_file, fake=True)
assert len(registry.spaces().spaces) == 2
assert len(registry.spaces().transcribers) == 1
try:
    Registry(Settings.models_file, fake=False)
except ValueError as error:
    assert 'not installed' in str(error), error
else:
    raise AssertionError('real models silently fell back')
""",
        cwd=directory,
    )


def test_plugin_wheels_discover_real_classes_and_preserve_fingerprints(installed, wheels):
    python, directory = installed
    plugins = [
        str(wheels / f"videohelper_ml_{name}-0.1.0-py3-none-any.whl")
        for name in ("wemm", "funasr", "clap")
    ]
    run(
        "uv",
        "pip",
        "install",
        "--offline",
        "--no-deps",
        "--python",
        str(python),
        *plugins,
        cwd=directory,
    )
    models = ML_ROOT / "models.local.yaml"
    run(
        str(python),
        "-I",
        "-c",
        """
import sys
from pathlib import Path
from app.core.plugins import load_adapter, adapter_distribution
from app.core.registry import Registry
for name, kind in [('wemm', 'embedder'), ('funasr', 'transcriber'), ('clap', 'embedder')]:
    adapter = load_adapter(name, kind)
    assert adapter.__module__ == 'videohelper_ml_' + name
    assert callable(adapter.prepare)
    assert adapter_distribution(name) == {'name': 'videohelper-ml-' + name, 'version': '0.1.0'}
    adapter.load = lambda self: None
registry = Registry(Path(sys.argv[1]), fake=False)
expected = {
    'wemm-2b@r1': '10930e4bb624c52978574c4e88d998cd663b7a67455ed33dd34cef2e5f6731c0',
    'paraformer-zh@r1': '8b5293fa83545a547f846c5878a83546b103de2601f844b026bf23d325a95e8f',
    'clap-htsat-unfused@r1': 'ed71d672a7feac1f1ded9e5934ae787f8af1716bc101015d5a2b5566c5f35da3',
}
for info in [*registry.spaces().spaces, *registry.spaces().transcribers]:
    assert info.fingerprint == 'sha256:' + expected[info.id], info
assert registry.gates['wemm-2b@r1', 'query'] is registry.gates['wemm-2b@r1', 'document']
""",
        str(models),
        cwd=directory,
    )
