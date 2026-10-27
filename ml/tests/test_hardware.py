import sys
from types import SimpleNamespace

import numpy as np
import pytest

from app.hardware import cuda_candidate, probe


@pytest.mark.parametrize(
    "available,broken,expected",
    [
        ({"mps", "cuda"}, set(), "mps"),
        ({"mps", "cuda"}, {"mps"}, "cuda"),
        ({"mps"}, {"mps"}, "cpu"),
        ({"cuda"}, {"cuda"}, "cpu"),
        (set(), set(), "cpu"),
    ],
)
def test_real_operation_required_before_selecting_device(monkeypatch, available, broken, expected):
    attempted = []

    class Tensor:
        def __init__(self, data):
            self.data = data

        def __matmul__(self, other):
            return Tensor(self.data @ other.data)

        def float(self):
            return self

        def cpu(self):
            return self

        def __getitem__(self, key):
            return self.data[key]

    def ones(shape, device, dtype):
        attempted.append((device, dtype))
        if device in broken:
            raise RuntimeError("device allocation failed")
        return Tensor(np.ones(shape))

    torch = SimpleNamespace(
        backends=SimpleNamespace(mps=SimpleNamespace(is_available=lambda: "mps" in available)),
        cuda=SimpleNamespace(is_available=lambda: "cuda" in available),
        ones=ones,
        float16="float16",
        float32="float32",
        isfinite=lambda tensor: np.isfinite(tensor.data),
    )
    monkeypatch.setitem(sys.modules, "torch", torch)
    result = probe()
    assert result["backend"] == expected
    assert attempted[-1] == (expected, "float32" if expected == "cpu" else "float16")
    if broken:
        assert "device allocation failed" in result["reason"]


def test_driver_absence_is_only_cpu_candidate(monkeypatch):
    def missing(name):
        raise OSError("no driver")

    monkeypatch.setattr("ctypes.CDLL", missing)
    assert cuda_candidate() is False
