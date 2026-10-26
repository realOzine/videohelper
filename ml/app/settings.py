import os
from dataclasses import dataclass
from pathlib import Path

_SOURCE_MODELS = Path(__file__).resolve().parents[1] / "models.yaml"
_DEFAULT_MODELS = (
    _SOURCE_MODELS
    if _SOURCE_MODELS.is_file()
    else (Path(__file__).resolve().parent / "resources" / "models.yaml")
)


@dataclass(frozen=True)
class Settings:
    token: str
    fake: bool = False
    models_file: Path = _DEFAULT_MODELS
    cache_dir: Path = Path("/var/cache/videohelper-ml")
    cache_max_bytes: int = 50_000_000_000
    s3_endpoint: str | None = None

    def __post_init__(self):
        if not self.token:
            raise ValueError("ML_INTERNAL_TOKEN must be set")
        if self.cache_max_bytes <= 0:
            raise ValueError("ML_CACHE_MAX_BYTES must be positive")

    @classmethod
    def from_env(cls):
        fake = os.getenv("ML_FAKE", "0")
        if fake not in ("0", "1"):
            raise ValueError("ML_FAKE must be 0 or 1")
        return cls(
            token=os.getenv("ML_INTERNAL_TOKEN", ""),
            fake=fake == "1",
            models_file=Path(os.getenv("ML_MODELS_FILE", cls.models_file)),
            cache_dir=Path(os.getenv("ML_CACHE_DIR", cls.cache_dir)),
            cache_max_bytes=int(os.getenv("ML_CACHE_MAX_BYTES", cls.cache_max_bytes)),
            s3_endpoint=os.getenv("ML_S3_ENDPOINT"),
        )
