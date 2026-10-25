import warnings
from pathlib import Path

from PIL import Image, ImageOps

from app.api.errors import MLError


def load_image(path: Path) -> Image.Image:
    try:
        with warnings.catch_warnings():
            warnings.simplefilter("error", Image.DecompressionBombWarning)
            with Image.open(path) as source:
                return ImageOps.exif_transpose(source).convert("RGB")
    except Exception as exc:
        raise MLError(422, "media_unreadable", "Cannot decode image") from exc
