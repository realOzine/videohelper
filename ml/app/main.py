import asyncio
import logging
import time
import uuid
from contextlib import asynccontextmanager

from fastapi import FastAPI, Request
from fastapi.exceptions import RequestValidationError

from app.api.errors import MLError, error_response, handle_error
from app.api.routes import router
from app.core.registry import Registry
from app.media.cache import MediaCache, S3Downloader
from app.settings import Settings

logger = logging.getLogger(__name__)


def create_app(settings: Settings | None = None, *, downloader=None) -> FastAPI:
    @asynccontextmanager
    async def lifespan(app: FastAPI):
        config = settings or Settings.from_env()
        app.state.settings = config
        app.state.ready = False
        app.state.cache = MediaCache(
            config.cache_dir,
            config.cache_max_bytes,
            downloader or S3Downloader(config.s3_endpoint),
            local_root=config.local_media_root,
        )
        app.state.registry = await asyncio.to_thread(
            Registry,
            config.models_file,
            fake=config.fake,
        )
        app.state.ready = True
        yield
        app.state.ready = False

    app = FastAPI(title="VideoHelper ML", version="1.0.0", lifespan=lifespan)
    app.state.ready = False
    app.add_exception_handler(MLError, handle_error)

    @app.exception_handler(RequestValidationError)
    async def invalid_request(request: Request, exc: RequestValidationError):
        return error_response(400, "invalid_input", "Request does not match the ML contract")

    @app.middleware("http")
    async def request_context(request: Request, call_next):
        received = time.monotonic()
        request_id = request.headers.get("X-Request-ID", "")
        request.state.deadline = None
        try:
            if request.url.path.startswith("/v1/"):
                if not request_id or len(request_id) > 256:
                    raise MLError(400, "invalid_input", "X-Request-ID is required (max 256 chars)")
                timeout = request.headers.get("X-Timeout-Ms")
                if timeout is not None:
                    if not timeout.isascii() or not timeout.isdecimal() or not timeout.strip("0"):
                        raise MLError(
                            400, "invalid_input", "X-Timeout-Ms must be positive milliseconds"
                        )
                    try:
                        request.state.deadline = received + int(timeout) / 1000
                    except (ValueError, OverflowError) as exc:
                        raise MLError(400, "invalid_input", "X-Timeout-Ms is too large") from exc
            response = await call_next(request)
        except MLError as exc:
            response = await handle_error(request, exc)
        except Exception:
            logger.exception("ML request failed request_id=%s", request_id)
            response = error_response(500, "internal", "Internal ML service error")
        response.headers["X-Request-ID"] = request_id or str(uuid.uuid4())
        return response

    @app.get("/healthz")
    async def healthz():
        return {"status": "ok"}

    @app.get("/readyz")
    async def readyz():
        if not app.state.ready:
            raise MLError(503, "not_ready", "Models are not ready")
        return {"status": "ok"}

    app.include_router(router)
    return app


app = create_app()
