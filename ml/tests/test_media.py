import wave

import numpy as np
import pytest

from app.adapters.fake import FakeTranscriber
from app.api.errors import MLError
from app.api.schemas import TranscriberInfo
from app.media import audio, video
from app.media.probe import duration_ms


def test_media_duration_frames_audio_and_short_end_window(media_file):
    assert duration_ms(media_file) == 2000
    frames = video.sample_frames(media_file, 0, 2000, 4, 16)
    assert frames.shape == (4, 16, 16, 3)
    assert frames[0].mean() < frames[-1].mean()
    end = video.sample_frames(media_file, 1900, 2000, 8, 16)
    assert end.shape == (8, 16, 16, 3)
    tiny = video.sample_frames(media_file, 1950, 1951, 4, None)
    assert tiny.shape == (4, 32, 32, 3)
    samples = audio.load(media_file, 500, 1000, 48000)
    assert samples.shape == (24000,)
    assert np.isfinite(samples).all()
    assert np.max(np.abs(samples)) > 0.1
    assert len(audio.load(media_file, 1990, 2000, 16000)) == 160


def test_unreadable_media(tmp_path):
    path = tmp_path / "bad"
    path.write_bytes(b"invalid")
    for operation in (
        lambda: duration_ms(path),
        lambda: video.sample_frames(path, 0, 100, 1, 16),
        lambda: audio.load(path, 0, 100, 16000),
    ):
        with pytest.raises(MLError) as error:
            operation()
        assert error.value.code == "media_unreadable"


def test_silent_audio_returns_empty_transcript(tmp_path):
    path = tmp_path / "silent.wav"
    with wave.open(str(path), "wb") as output:
        output.setnchannels(1)
        output.setsampwidth(2)
        output.setframerate(16000)
        output.writeframes(np.zeros(16000, np.int16).tobytes())
    info = TranscriberInfo(
        id="fake", languages=["zh"], max_window_ms=900000, fingerprint="sha256:" + "0" * 64
    )
    transcript = FakeTranscriber(info, {}).transcribe(path, "zh", 100, 900)
    assert transcript.duration_ms == 1000
    assert transcript.sentences == []


def test_image_decode_converts_rgb_and_rejects_invalid(tmp_path):
    from PIL import Image

    from app.media.image import load_image

    path = tmp_path / "image.png"
    Image.new("RGBA", (12, 8), (255, 0, 0, 255)).save(path)
    assert load_image(path).mode == "RGB"
    path.write_bytes(b"broken")
    with pytest.raises(MLError) as error:
        load_image(path)
    assert error.value.code == "media_unreadable"
