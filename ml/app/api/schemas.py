from typing import Annotated, Literal

from pydantic import BaseModel, ConfigDict, Field, model_validator

Role = Literal["query", "document"]
InputKind = Literal["text", "video", "audio", "image"]
PositiveInt = Annotated[int, Field(strict=True, gt=0)]
Timestamp = Annotated[int, Field(strict=True, ge=0)]
Identifier = Annotated[str, Field(min_length=1, pattern=r"^[a-z0-9@._-]+$")]
MediaURI = Annotated[
    str,
    Field(
        pattern=r"^local://media/[A-Za-z0-9_-][A-Za-z0-9._-]*"
        r"(?:/[A-Za-z0-9_-][A-Za-z0-9._-]*)*$"
    ),
]
Fingerprint = Annotated[str, Field(pattern=r"^sha256:[0-9a-f]{64}$")]


class Schema(BaseModel):
    model_config = ConfigDict(extra="forbid", allow_inf_nan=False)


class ImageRef(Schema):
    uri: MediaURI


class MediaRef(ImageRef):
    start_ms: Timestamp
    end_ms: PositiveInt

    @model_validator(mode="after")
    def valid_window(self):
        if self.start_ms >= self.end_ms:
            raise ValueError("start_ms must be less than end_ms")
        return self


class EmbedInput(Schema):
    text: Annotated[str, Field(min_length=1)] | None = None
    video: MediaRef | None = None
    audio: MediaRef | None = None
    image: ImageRef | None = None

    @model_validator(mode="after")
    def exactly_one_input(self):
        if len(self.model_fields_set) != 1 or all(
            value is None for value in (self.text, self.video, self.audio, self.image)
        ):
            raise ValueError("each input must contain exactly one of text, video, audio, image")
        return self

    @property
    def kind(self) -> InputKind:
        return next(iter(self.model_fields_set))


class EmbedRequest(Schema):
    space: Identifier
    role: Role
    inputs: Annotated[list[EmbedInput], Field(min_length=1)]


class SpaceInfo(Schema):
    id: Identifier
    dim: PositiveInt
    metric: Literal["cosine", "ip", "l2"]
    query_inputs: Annotated[list[InputKind], Field(min_length=1)]
    doc_inputs: Annotated[list[InputKind], Field(min_length=1)]
    max_span_ms: PositiveInt | None
    max_batch: PositiveInt
    fingerprint: Fingerprint


class TranscriberInfo(Schema):
    id: Identifier
    languages: Annotated[list[str], Field(min_length=1)]
    max_window_ms: PositiveInt
    fingerprint: Fingerprint


class SpacesResponse(Schema):
    spaces: list[SpaceInfo]
    transcribers: list[TranscriberInfo]


class RuntimeModel(Schema):
    id: Identifier
    kind: Literal["embedder", "transcriber"]
    adapter: str
    distribution: dict[str, str] | None
    device: str
    dtype: str
    weights: str
    revision: str | None
    weights_digest: str | None
    fingerprint: Fingerprint


class RuntimeResponse(Schema):
    runtime_version: Literal[1]
    service_version: str
    python_version: str
    packages: dict[str, str]
    fake: bool
    models: list[RuntimeModel]


class EmbedResponse(Schema):
    space: Identifier
    fingerprint: Fingerprint
    dim: PositiveInt
    vectors: list[list[float]]


class TranscribeRequest(MediaRef):
    model: Identifier
    language: Annotated[str, Field(min_length=1)]


class Word(Schema):
    w: str
    s: Timestamp
    e: PositiveInt


class Sentence(Schema):
    start_ms: Timestamp
    end_ms: PositiveInt
    text: str
    words: list[Word]


class Transcript(Schema):
    language: str
    duration_ms: Timestamp
    sentences: list[Sentence]


class TranscribeResponse(Transcript):
    model: Identifier
    fingerprint: Fingerprint


class ErrorDetail(Schema):
    code: Literal[
        "unauthorized",
        "invalid_input",
        "unknown_space",
        "unknown_model",
        "media_unreadable",
        "busy",
        "not_ready",
        "deadline_exceeded",
        "internal",
    ]
    message: str


class ErrorBody(Schema):
    error: ErrorDetail
