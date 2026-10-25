import hashlib
from pathlib import Path

import numpy as np

from app.adapters.base import ImageInput, MediaInput, TextInput
from app.api.schemas import Role, Sentence, SpaceInfo, TranscriberInfo, Transcript, Word
from app.media import audio, video
from app.media.image import load_image
from app.media.probe import duration_ms


class FakeEmbedder:
    def __init__(self, info: SpaceInfo, preprocess: dict):
        self.info = info
        self.preprocess = preprocess

    def load(self):
        pass

    def embed(self, inputs: list[TextInput | MediaInput | ImageInput], role: Role) -> np.ndarray:
        vectors = []
        for item in inputs:
            if isinstance(item, TextInput):
                content = item.text.encode()
            elif isinstance(item, ImageInput):
                content = load_image(item.path).tobytes()
            elif item.kind == "video":
                content = video.sample_frames(
                    item.path,
                    item.start_ms,
                    item.end_ms,
                    self.preprocess.get("frames_per_clip", 8),
                    self.preprocess.get("image_size", 224),
                ).tobytes()
            else:
                content = audio.load(
                    item.path,
                    item.start_ms,
                    item.end_ms,
                    self.preprocess.get("sample_rate", 48000),
                ).tobytes()
            # Role is deliberately represented so query/document dispatch is testable.
            raw = hashlib.shake_256(role.encode() + b"\0" + content).digest(self.info.dim * 4)
            vector = np.frombuffer(raw, dtype="<u4").astype(np.float64) / (2**31) - 1
            if self.info.metric == "cosine":
                vector /= np.linalg.norm(vector)
            vectors.append(vector.astype(np.float32))
        return np.stack(vectors)


class FakeTranscriber:
    def __init__(self, info: TranscriberInfo, preprocess: dict):
        self.info = info
        self.preprocess = preprocess

    def load(self):
        pass

    def transcribe(self, path: Path, language: str, start_ms: int, end_ms: int) -> Transcript:
        duration = duration_ms(path)
        samples = audio.load(path, start_ms, min(end_ms, duration), 16000)
        sentences = []
        if np.any(np.abs(samples) > 1e-6):
            end = min(start_ms + 1000, end_ms, duration)
            sentences = [
                Sentence(
                    start_ms=start_ms,
                    end_ms=end,
                    text="假模型转录",
                    words=[Word(w="假模型转录", s=start_ms, e=end)],
                )
            ]
        return Transcript(language=language, duration_ms=duration, sentences=sentences)
