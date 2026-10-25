from contextlib import ExitStack

import numpy as np
from fastapi import APIRouter, Depends, Request

from app.adapters.base import ImageInput, MediaInput, TextInput
from app.api.auth import authenticate
from app.api.errors import MLError
from app.api.schemas import (
    EmbedRequest,
    EmbedResponse,
    SpacesResponse,
    TranscribeRequest,
    TranscribeResponse,
)
from app.media.probe import duration_ms

router = APIRouter(prefix="/v1", dependencies=[Depends(authenticate)])


def ready(request: Request):
    if not request.app.state.ready:
        raise MLError(503, "not_ready", "Models are not ready")
    return request.app.state.registry


@router.get("/spaces", response_model=SpacesResponse)
async def spaces(request: Request):
    return ready(request).spaces()


@router.post("/embed", response_model=EmbedResponse)
async def embed(body: EmbedRequest, request: Request):
    registry = ready(request)
    adapter = registry.embedders.get(body.space)
    if adapter is None:
        raise MLError(404, "unknown_space", "Space is not loaded on this instance")
    info = adapter.info
    if len(body.inputs) > info.max_batch:
        raise MLError(400, "invalid_input", "Batch exceeds max_batch")
    allowed = info.query_inputs if body.role == "query" else info.doc_inputs
    for item in body.inputs:
        if item.kind not in allowed:
            raise MLError(400, "invalid_input", "Input type is not supported for this role")
        media = item.video or item.audio
        if media is not None and media.end_ms - media.start_ms > info.max_span_ms:
            raise MLError(400, "invalid_input", "Media window exceeds max_span_ms")

    def infer():
        with ExitStack() as stack:
            files = {}
            inputs = []
            for item in body.inputs:
                if item.text is not None:
                    inputs.append(TextInput(item.text))
                    continue
                media = item.video or item.audio or item.image
                if media.uri not in files:
                    files[media.uri] = stack.enter_context(
                        request.app.state.cache.acquire(media.uri)
                    )
                path = files[media.uri]
                if item.image is not None:
                    inputs.append(ImageInput(path))
                    continue
                duration = duration_ms(path)
                end_ms = min(media.end_ms, duration)
                if media.start_ms >= end_ms:
                    raise MLError(422, "media_unreadable", "No media in requested window")
                inputs.append(MediaInput(item.kind, path, media.start_ms, end_ms))
            vectors = np.asarray(adapter.embed(inputs, body.role))
            if vectors.shape != (len(inputs), info.dim) or not np.isfinite(vectors).all():
                raise MLError(500, "internal", "Invalid embedding output")
            if info.metric == "cosine" and not np.allclose(
                np.linalg.norm(vectors, axis=1),
                1,
                atol=1e-4,
            ):
                raise MLError(500, "internal", "Embedding is not L2 normalized")
            return EmbedResponse(
                space=info.id,
                fingerprint=info.fingerprint,
                dim=info.dim,
                vectors=vectors.tolist(),
            )

    return await registry.gates[body.space, body.role].run(infer, request.state.deadline)


@router.post("/transcribe", response_model=TranscribeResponse)
async def transcribe(body: TranscribeRequest, request: Request):
    registry = ready(request)
    adapter = registry.transcribers.get(body.model)
    if adapter is None:
        raise MLError(404, "unknown_model", "Transcriber is not loaded on this instance")
    info = adapter.info
    if body.language not in info.languages:
        raise MLError(400, "invalid_input", "Unsupported language")
    if body.end_ms - body.start_ms > info.max_window_ms:
        raise MLError(400, "invalid_input", "Window exceeds max_window_ms")

    def infer():
        with request.app.state.cache.acquire(body.uri) as path:
            duration = duration_ms(path)
            end_ms = min(body.end_ms, duration)
            if body.start_ms >= end_ms:
                raise MLError(422, "media_unreadable", "No media in requested window")
            transcript = adapter.transcribe(path, body.language, body.start_ms, end_ms)
            previous = body.start_ms
            valid = transcript.duration_ms == duration and transcript.language in info.languages
            for sentence in transcript.sentences:
                valid &= previous <= sentence.start_ms < sentence.end_ms <= end_ms
                previous = sentence.end_ms
                word_end = sentence.start_ms
                for word in sentence.words:
                    valid &= word_end <= word.s < word.e <= sentence.end_ms
                    word_end = word.e
            if not valid:
                raise MLError(500, "internal", "Invalid transcript output")
            return TranscribeResponse(
                model=info.id,
                fingerprint=info.fingerprint,
                **transcript.model_dump(),
            )

    return await registry.gates[body.model, "transcribe"].run(infer, request.state.deadline)
