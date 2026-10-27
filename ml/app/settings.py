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
    local_media_root: Path | None = None

    def __post_init__(self):
        if not self.token:
            raise ValueError("ML_INTERNAL_TOKEN must be set")
        if self.local_media_root is None or not self.local_media_root.is_absolute():
            raise ValueError("ML_LOCAL_MEDIA_ROOT must be an absolute managed directory")

    @classmethod
    def from_env(cls):
        fake = os.getenv("ML_FAKE", "0")
        if fake not in ("0", "1"):
            raise ValueError("ML_FAKE must be 0 or 1")
        return cls(
            token=os.getenv("ML_INTERNAL_TOKEN", ""),
            fake=fake == "1",
            models_file=Path(os.getenv("ML_MODELS_FILE", cls.models_file)),
            local_media_root=(
                Path(os.environ["ML_LOCAL_MEDIA_ROOT"])
                if os.getenv("ML_LOCAL_MEDIA_ROOT")
                else None
            ),
        )
