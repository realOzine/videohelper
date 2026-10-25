from pathlib import Path

import av
import numpy as np

from app.api.errors import MLError


def load(path: Path, start_ms: int, end_ms: int, sample_rate: int) -> np.ndarray:
    try:
        with av.open(str(path)) as container:
            stream = container.streams.audio[0]
            container.seek(int(start_ms / 1000 / stream.time_base), stream=stream)
            resampler = av.AudioResampler(format="fltp", layout="mono", rate=sample_rate)
            output = np.zeros(int(np.ceil((end_ms - start_ms) * sample_rate / 1000)), np.float32)
            found = False

            def append(frame):
                nonlocal found
                if frame.pts is None:
                    return
                offset = round(float(frame.pts * frame.time_base) * sample_rate)
                offset -= round(start_ms * sample_rate / 1000)
                samples = frame.to_ndarray().reshape(-1)
                left, right = max(0, offset), min(len(output), offset + len(samples))
                if left < right:
                    output[left:right] = samples[left - offset : right - offset]
                    found = True

            for frame in container.decode(stream):
                if frame.pts is not None and frame.pts * frame.time_base * 1000 >= end_ms:
                    break
                for converted in resampler.resample(frame):
                    append(converted)
            for converted in resampler.resample(None):
                append(converted)
            if not found:
                raise ValueError("No audio samples in window")
            return output
    except Exception as exc:
        raise MLError(422, "media_unreadable", "Cannot decode audio window") from exc
