"""Select a runtime candidate, then prove the backend using the installed Torch."""

import ctypes
import json
import platform
import sys


def cuda_candidate() -> bool:
    try:
        library = "nvcuda.dll" if platform.system() == "Windows" else "libcuda.so.1"
        driver = ctypes.CDLL(library)
        count, version = ctypes.c_int(), ctypes.c_int()
        return (
            driver.cuInit(0) == 0
            and driver.cuDeviceGetCount(ctypes.byref(count)) == 0
            and count.value > 0
            and driver.cuDriverGetVersion(ctypes.byref(version)) == 0
            and version.value >= 13000
        )
    except (OSError, AttributeError):
        return False


def probe() -> dict[str, str]:
    import torch

    errors = []
    for backend, available, dtype in [
        ("mps", torch.backends.mps.is_available, torch.float16),
        ("cuda", torch.cuda.is_available, torch.float16),
        ("cpu", lambda: True, torch.float32),
    ]:
        try:
            if not available():
                continue
            a = torch.ones((8, 8), device=backend, dtype=dtype)
            result = (a @ a).float().cpu()
            if not torch.isfinite(result).all().item() or result[0, 0].item() != 8:
                raise RuntimeError("Backend produced an invalid result")
            reason = "; ".join(errors)
            if backend == "cpu" and not reason:
                reason = "未检测到可运行的 MPS / CUDA，使用 CPU"
            return {"backend": backend, "reason": reason}
        except Exception as exc:
            errors.append(backend + ": " + str(exc)[:1000])
    raise RuntimeError("; ".join(errors))


if __name__ == "__main__":
    if sys.argv[1:] == ["--cuda-candidate"]:
        print("cuda" if cuda_candidate() else "cpu")
    else:
        print(json.dumps(probe()))
