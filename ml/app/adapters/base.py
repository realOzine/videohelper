from dataclasses import dataclass
from pathlib import Path
from typing import Literal, Protocol

import numpy as np

from app.api.schemas import Role, SpaceInfo, TranscriberInfo, Transcript


@dataclass(frozen=True)
class TextInput:
    text: str


@dataclass(frozen=True)
class MediaInput:
    kind: Literal["video", "audio"]
    path: Path
    start_ms: int
    end_ms: int


class Embedder(Protocol):
    info: SpaceInfo

    def load(self) -> None: ...
    def embed(self, inputs: list[TextInput | MediaInput], role: Role) -> np.ndarray: ...


class Transcriber(Protocol):
    info: TranscriberInfo

    def load(self) -> None: ...
    def transcribe(self, path: Path, language: str, start_ms: int, end_ms: int) -> Transcript: ...
