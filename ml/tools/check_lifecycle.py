"""Build a core wheel and check its foreground lifecycle without model downloads."""

import json
import os
import socket
import subprocess
import tempfile
import time
import urllib.error
import urllib.request
from pathlib import Path

ML_ROOT = Path(__file__).resolve().parents[1]


def main():
    with tempfile.TemporaryDirectory(prefix="videohelper-ml-lifecycle-") as directory:
        root = Path(directory)
        environment = {**os.environ, "PYTHONPATH": "", "UV_NO_SYNC": "1"}
        subprocess.run(
            ["uv", "build", "--wheel", "--out-dir", str(root / "wheels"), str(ML_ROOT)],
            check=True,
            env=environment,
        )
        subprocess.run(["uv", "venv", "--python", "3.12", str(root / "venv")], check=True)
        python = root / "venv/bin/python"
        subprocess.run(
            [
                "uv",
                "pip",
                "install",
                "--python",
                str(python),
                str(next((root / "wheels").glob("*.whl"))),
            ],
            check=True,
            env=environment,
        )
        installed = subprocess.check_output(
            [str(python), "-c", "import app; print(app.__file__)"],
            cwd=root,
            env=environment,
            text=True,
        )
        assert str(root / "venv") in installed and "site-packages" in installed, installed
        manifest = root / "models.yaml"
        manifest.write_bytes((ML_ROOT / "models.yaml").read_bytes())
        environment.update(
            ML_FAKE="1", ML_INTERNAL_TOKEN="lifecycle-secret", ML_CACHE_DIR=str(root / "cache")
        )
        with socket.socket() as sock:
            sock.bind(("127.0.0.1", 0))
            port = sock.getsockname()[1]
        command = [
            str(root / "venv/bin/videohelper-ml"),
            "serve",
            "--models",
            str(manifest),
            "--port",
            str(port),
        ]
        base = f"http://127.0.0.1:{port}"
        with (root / "server.log").open("w+") as log:
            process = subprocess.Popen(command, cwd=root, env=environment, stdout=log, stderr=log)
            try:
                for _ in range(300):
                    assert process.poll() is None, "Installed service exited before readiness"
                    try:
                        with urllib.request.urlopen(base + "/readyz", timeout=0.2):
                            break
                    except (urllib.error.URLError, TimeoutError):
                        time.sleep(0.1)
                else:
                    raise AssertionError("Installed service did not become ready")
                headers = {"X-Request-ID": "lifecycle", "X-Internal-Token": "lifecycle-secret"}
                request = urllib.request.Request(base + "/v1/runtime", headers=headers)
                with urllib.request.urlopen(request, timeout=2) as response:
                    runtime = json.load(response)
                assert runtime["service_version"] == "0.1.0" and runtime["fake"] is True
                assert len(runtime["models"]) == 3
                request = urllib.request.Request(
                    base + "/v1/runtime", headers={"X-Request-ID": "unauthenticated"}
                )
                try:
                    urllib.request.urlopen(request, timeout=2)
                except urllib.error.HTTPError as error:
                    assert error.code == 401
                else:
                    raise AssertionError("Runtime metadata accepted an unauthenticated request")
                body = {"space": "wemm-2b@r1", "role": "query", "inputs": [{"text": "hello"}]}
                request = urllib.request.Request(
                    base + "/v1/embed",
                    data=json.dumps(body).encode(),
                    headers={**headers, "Content-Type": "application/json"},
                )
                with urllib.request.urlopen(request, timeout=2) as response:
                    assert len(json.load(response)["vectors"][0]) == 1024
            except BaseException:
                log.flush()
                log.seek(0)
                print(log.read())
                raise
            finally:
                process.terminate()
                try:
                    process.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait(timeout=5)
        assert process.returncode is not None
        with socket.socket() as sock:
            assert sock.connect_ex(("127.0.0.1", port)) != 0, (
                "Stopped service still accepts traffic"
            )
        refused = subprocess.run(
            command,
            cwd=root,
            env={**environment, "ML_FAKE": "0"},
            capture_output=True,
            text=True,
            timeout=10,
        )
        assert refused.returncode != 0, "Service started without its required plugin"
        assert "Real adapter 'wemm' is not installed" in refused.stderr, refused.stderr
        assert "Install its plugin package" in refused.stderr, refused.stderr
        invalid = subprocess.run(
            command,
            cwd=root,
            env={**environment, "ML_INTERNAL_TOKEN": ""},
            capture_output=True,
            text=True,
            timeout=10,
        )
        assert invalid.returncode != 0 and "ML_INTERNAL_TOKEN" in invalid.stderr
        print("Installed wheel lifecycle passed; all temporary server processes stopped")


if __name__ == "__main__":
    main()
