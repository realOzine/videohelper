from pathlib import Path

import av
import numpy as np

from app.api.errors import MLError


def sample_frames(
    path: Path,
    start_ms: int,
    end_ms: int,
    count: int,
    image_size: int | None,
) -> np.ndarray:
    try:
        with av.open(str(path)) as container:
            stream = container.streams.video[0]
            container.seek(int(start_ms / 1000 / stream.time_base), stream=stream)
            targets = np.linspace(start_ms, end_ms, count, endpoint=False)

            def pixels(frame):
                if image_size is not None:
                    frame = frame.reformat(image_size, image_size)
                return frame.to_ndarray(format="rgb24")

            frames = []
            target = 0
            last = None
            preceding = None
            for frame in container.decode(stream):
                if frame.pts is None:
                    continue
                timestamp = float(frame.pts * frame.time_base * 1000)
                if timestamp < start_ms:
                    preceding = frame
                    continue
                if timestamp >= end_ms:
                    break
                last = frame
                while target < count and timestamp >= targets[target]:
                    frames.append(pixels(frame))
                    target += 1
                if target == count:
                    break
            if last is None:
                # A sub-frame window still displays the preceding decoded frame.
                last = preceding
            if last is None:
                raise ValueError("No video frames in window")
            while len(frames) < count:
                frames.append(pixels(last))
            return np.stack(frames)
    except Exception as exc:
        raise MLError(422, "media_unreadable", "Cannot decode video window") from exc
