import re
from pathlib import Path

from app.api.errors import MLError


def local_media_path(uri: str, root: Path | None) -> Path:
    if root is None or not re.fullmatch(
        r"local://media/[A-Za-z0-9_-][A-Za-z0-9._-]*(?:/[A-Za-z0-9_-][A-Za-z0-9._-]*)*",
        uri,
    ):
        raise MLError(400, "invalid_input", "Local media is disabled or has an invalid key")
    try:
        root = root.resolve(strict=True)
        path = root
        for part in uri.removeprefix("local://media/").split("/"):
            path = path / part
            if path.is_symlink():
                raise ValueError("Symbolic links are not local media")
        resolved = path.resolve(strict=True)
        if (
            not resolved.is_relative_to(root)
            or not resolved.is_file()
            or resolved.stat().st_size == 0
        ):
            raise ValueError("Expected a nonempty regular file inside the media root")
        return resolved
    except (OSError, ValueError) as exc:
        raise MLError(422, "media_unreadable", "Cannot read local media") from exc
