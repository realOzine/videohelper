from pathlib import Path
from types import MappingProxyType
from typing import Any, Literal

import yaml
from pydantic import BaseModel, ConfigDict, Field, model_validator

from app.adapters.base import Embedder, Transcriber
from app.adapters.fake import FakeEmbedder, FakeTranscriber
from app.api.schemas import SpaceInfo, SpacesResponse, TranscriberInfo
from app.core.fingerprint import fingerprint
from app.core.gate import Gate
from app.core.plugins import load_adapter


class ModelConfig(BaseModel):
    model_config = ConfigDict(extra="forbid")
    id: str
    kind: Literal["embedder", "transcriber"]
    adapter: str
    weights: str
    revision: str | None = None
    weights_digest: str | None = None
    dtype: str
    device: str = "cpu"
    preprocess: dict[str, Any] = Field(default_factory=dict)
    max_concurrency: int = Field(default=1, gt=0)
    queue_limit: int = Field(default=64, ge=0)
    query_concurrency: int = Field(default=1, gt=0)
    query_queue_limit: int = Field(default=8, ge=0)
    dim: int | None = Field(default=None, gt=0)
    metric: Literal["cosine", "ip", "l2"] = "cosine"
    query_inputs: list[Literal["text", "video", "audio", "image"]] = Field(
        default_factory=lambda: ["text"]
    )
    doc_inputs: list[Literal["text", "video", "audio", "image"]] = Field(default_factory=list)
    max_batch: int = Field(default=64, gt=0)
    max_span_ms: int | None = Field(default=None, gt=0)
    max_window_ms: int = Field(default=900000, gt=0)
    languages: list[str] = Field(default_factory=lambda: ["zh"])

    @model_validator(mode="after")
    def immutable_weights(self):
        if not self.revision and not self.weights_digest:
            raise ValueError("revision or weights_digest is required")
        if self.kind == "embedder" and (
            not self.dim or not self.doc_inputs or not self.query_inputs
        ):
            raise ValueError("embedder requires dim, query_inputs and doc_inputs")
        if (
            self.kind == "embedder"
            and any(kind in ("video", "audio") for kind in self.query_inputs + self.doc_inputs)
            and self.max_span_ms is None
        ):
            raise ValueError("media embedder requires max_span_ms")
        return self


class Manifest(BaseModel):
    model_config = ConfigDict(extra="forbid")
    models: list[ModelConfig] = Field(min_length=1)


class Registry:
    def __init__(self, path: Path, *, fake: bool):
        self.embedders: dict[str, Embedder] = {}
        self.transcribers: dict[str, Transcriber] = {}
        self.gates: dict[tuple[str, str], Gate] = {}
        manifest = Manifest.model_validate(yaml.safe_load(path.read_text()))
        ids = [config.id for config in manifest.models]
        if len(set(ids)) != len(ids):
            raise ValueError("Duplicate model id")
        self.configs = MappingProxyType({config.id: config for config in manifest.models})
        adapter_types = (
            {config.id: load_adapter(config.adapter, config.kind) for config in manifest.models}
            if not fake
            else {}
        )
        for config in manifest.models:
            digest = fingerprint(config.model_dump(), fake=fake)
            if config.kind == "embedder":
                info = SpaceInfo(
                    id=config.id,
                    dim=config.dim,
                    metric=config.metric,
                    query_inputs=config.query_inputs,
                    doc_inputs=config.doc_inputs,
                    max_span_ms=config.max_span_ms,
                    max_batch=config.max_batch,
                    fingerprint=digest,
                )
                if fake:
                    adapter = FakeEmbedder(info, config.preprocess)
                else:
                    adapter = adapter_types[config.id](info, config.model_dump())
                adapter.load()
                self.embedders[config.id] = adapter
                self.gates[config.id, "document"] = Gate(config.max_concurrency, config.queue_limit)
                if fake or not adapter_types[config.id].shared_query_gate:
                    self.gates[config.id, "query"] = Gate(
                        config.query_concurrency,
                        config.query_queue_limit,
                    )
                else:
                    self.gates[config.id, "query"] = self.gates[config.id, "document"]
            else:
                info = TranscriberInfo(
                    id=config.id,
                    languages=config.languages,
                    max_window_ms=config.max_window_ms,
                    fingerprint=digest,
                )
                if fake:
                    adapter = FakeTranscriber(info, config.preprocess)
                else:
                    adapter = adapter_types[config.id](info, config.model_dump())
                adapter.load()
                self.transcribers[config.id] = adapter
                self.gates[config.id, "transcribe"] = Gate(
                    config.max_concurrency, config.queue_limit
                )

    def spaces(self) -> SpacesResponse:
        return SpacesResponse(
            spaces=[adapter.info for adapter in self.embedders.values()],
            transcribers=[adapter.info for adapter in self.transcribers.values()],
        )
