from pathlib import Path

import av

from app.api.errors import MLError


def duration_ms(path: Path) -> int:
    try:
        with av.open(str(path)) as container:
            if container.duration is not None:
                return int(container.duration * 1000 / av.time_base)
            durations = [
                int(stream.duration * stream.time_base * 1000)
                for stream in container.streams
                if stream.duration is not None and stream.time_base is not None
            ]
            if not durations:
                raise ValueError("Media has no duration")
            return max(durations)
    except Exception as exc:
        raise MLError(422, "media_unreadable", "Cannot decode media duration") from exc
