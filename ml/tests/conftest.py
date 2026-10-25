from fractions import Fraction
from pathlib import Path

import av
import numpy as np
import pytest
from fastapi.testclient import TestClient

from app.main import create_app
from app.settings import Settings

HEADERS = {"X-Internal-Token": "test-secret", "X-Request-ID": "test-request"}


@pytest.fixture
def media_file(tmp_path):
    path = tmp_path / "sample.mkv"
    with av.open(str(path), "w") as container:
        video = container.add_stream("mpeg4", rate=10)
        video.width = video.height = 32
        video.pix_fmt = "yuv420p"
        audio = container.add_stream("pcm_s16le", rate=16000)
        audio.layout = "mono"
        for index in range(20):
            image = np.full((32, 32, 3), index * 10, dtype=np.uint8)
            frame = av.VideoFrame.from_ndarray(image, format="rgb24")
            frame.pts = index
            for packet in video.encode(frame):
                container.mux(packet)
        for packet in video.encode():
            container.mux(packet)
        samples = (np.sin(np.arange(32000) * 2 * np.pi * 440 / 16000) * 10000).astype(np.int16)
        frame = av.AudioFrame.from_ndarray(samples.reshape(1, -1), format="s16", layout="mono")
        frame.sample_rate = 16000
        frame.time_base = Fraction(1, 16000)
        frame.pts = 0
        for packet in audio.encode(frame):
            container.mux(packet)
        for packet in audio.encode():
            container.mux(packet)
    return path


@pytest.fixture
def client(tmp_path, media_file):
    settings = Settings(token="test-secret", fake=True, cache_dir=tmp_path / "cache")

    def download(uri, write, max_bytes):
        write(media_file.read_bytes())

    app = create_app(settings, downloader=download)
    with TestClient(app, headers=HEADERS) as client:
        yield client


@pytest.fixture
def contract():
    import yaml

    return yaml.safe_load((Path(__file__).parents[2] / "contracts/ml-v1.yaml").read_text())
