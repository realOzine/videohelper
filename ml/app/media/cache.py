import hashlib
import os
import re
import tempfile
import threading
from collections.abc import Callable, Iterator
from contextlib import contextmanager
from pathlib import Path
from urllib.parse import urlsplit

from app.api.errors import MLError


def split_s3_uri(uri: str) -> tuple[str, str]:
    if not re.fullmatch(r"s3://[a-z0-9][a-z0-9.-]*/[^?#]+", uri):
        raise MLError(400, "invalid_input", "Expected s3://bucket/key")
    parsed = urlsplit(uri)
    if (
        parsed.scheme != "s3"
        or not parsed.netloc
        or len(parsed.path) <= 1
        or parsed.query
        or parsed.fragment
        or parsed.username
    ):
        raise MLError(400, "invalid_input", "Expected s3://bucket/key")
    return parsed.netloc, parsed.path[1:]


class S3Downloader:
    def __init__(self, endpoint: str | None):
        self.endpoint = endpoint
        self.client = None
        self.lock = threading.Lock()

    def __call__(self, uri: str, write: Callable[[bytes], None], max_bytes: int):
        import boto3
        from botocore.config import Config

        bucket, key = split_s3_uri(uri)
        with self.lock:
            if self.client is None:
                self.client = boto3.client(
                    "s3",
                    endpoint_url=self.endpoint,
                    region_name=os.getenv("AWS_REGION"),
                    config=Config(connect_timeout=5, read_timeout=30, retries={"max_attempts": 2}),
                )
        response = self.client.get_object(Bucket=bucket, Key=key)
        body = response["Body"]
        try:
            if response.get("ContentLength", 0) > max_bytes:
                raise OSError("Media exceeds cache capacity")
            size = 0
            for chunk in body.iter_chunks(chunk_size=1024 * 1024):
                size += len(chunk)
                if size > max_bytes:
                    raise OSError("Media exceeds cache capacity")
                write(chunk)
        finally:
            body.close()


class MediaCache:
    def __init__(self, directory: Path, max_bytes: int, download: Callable):
        self.directory = directory
        self.max_bytes = max_bytes
        self.download = download
        # shortcut: coordination is process-local; use a separate cache directory per worker.
        self.condition = threading.Condition()
        self.downloading: set[str] = set()
        self.staging: dict[str, int] = {}
        self.pins: dict[Path, int] = {}
        self.directory.mkdir(parents=True, exist_ok=True)
        with self.condition:
            self._evict(0)

    def _evict(self, incoming: int):
        files = sorted(self.directory.glob("*.media"), key=lambda path: path.stat().st_mtime_ns)
        total = sum(path.stat().st_size for path in files) + sum(self.staging.values())
        for path in files:
            if total + incoming <= self.max_bytes:
                return
            if not self.pins.get(path):
                total -= path.stat().st_size
                path.unlink()
        if total + incoming > self.max_bytes:
            raise MLError(503, "busy", "Media cache is in use")

    @contextmanager
    def acquire(self, uri: str) -> Iterator[Path]:
        split_s3_uri(uri)
        key = hashlib.sha256(uri.encode()).hexdigest()
        path = self.directory / (key + ".media")
        with self.condition:
            while key in self.downloading:
                self.condition.wait()
            if path.exists():
                os.utime(path, None)
                self.pins[path] = self.pins.get(path, 0) + 1
                cached = True
            else:
                self.downloading.add(key)
                self.staging[key] = 0
                cached = False
        if not cached:
            temporary = None
            try:
                fd, name = tempfile.mkstemp(prefix=key + ".", suffix=".part", dir=self.directory)
                os.close(fd)
                temporary = Path(name)
                with temporary.open("wb") as output:

                    def write(chunk):
                        with self.condition:
                            if self.staging[key] + len(chunk) > self.max_bytes:
                                raise OSError("Media exceeds cache capacity")
                            self._evict(len(chunk))
                            self.staging[key] += len(chunk)
                        output.write(chunk)

                    self.download(uri, write, self.max_bytes)
                size = temporary.stat().st_size
                if size == 0 or size > self.max_bytes:
                    raise OSError("Empty media or media exceeds cache capacity")
                with self.condition:
                    temporary.replace(path)
                    del self.staging[key]
                    self.pins[path] = 1
            except MLError:
                raise
            except Exception as exc:
                raise MLError(422, "media_unreadable", "Cannot read media from S3") from exc
            finally:
                if temporary is not None:
                    temporary.unlink(missing_ok=True)
                with self.condition:
                    self.staging.pop(key, None)
                    self.downloading.remove(key)
                    self.condition.notify_all()
        try:
            yield path
        finally:
            with self.condition:
                self.pins[path] -= 1
                if not self.pins[path]:
                    del self.pins[path]
                self.condition.notify_all()
