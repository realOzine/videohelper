import threading
from concurrent.futures import ThreadPoolExecutor

import pytest

from app.api.errors import MLError
from app.media.cache import MediaCache, split_s3_uri


def test_concurrent_same_uri_downloads_once_and_is_pinned(tmp_path):
    count = 0
    release = threading.Event()
    entered = threading.Event()

    def download(uri, write, limit):
        nonlocal count
        count += 1
        entered.set()
        release.wait(2)
        write(b"data")

    cache = MediaCache(tmp_path, 8, download)

    def fetch():
        with cache.acquire("s3://bucket/key") as path:
            return path.read_bytes()

    with ThreadPoolExecutor(2) as pool:
        first = pool.submit(fetch)
        assert entered.wait(1)
        second = pool.submit(fetch)
        release.set()
        assert first.result() == second.result() == b"data"
    assert count == 1
    assert not cache.pins
    assert not list(tmp_path.glob("*.part"))


def test_lru_eviction_does_not_remove_active_file(tmp_path):
    def download(uri, write, limit):
        write(b"1234")

    cache = MediaCache(tmp_path, 8, download)
    with cache.acquire("s3://bucket/a") as a:
        with cache.acquire("s3://bucket/b") as b:
            with pytest.raises(MLError) as error:
                with cache.acquire("s3://bucket/c"):
                    pass
            assert error.value.code == "busy"
            assert a.exists() and b.exists()
        with cache.acquire("s3://bucket/c"):
            assert a.exists()
            assert not b.exists()
    assert sum(path.stat().st_size for path in tmp_path.glob("*.media")) <= 8


def test_failed_download_is_removed_and_retry_works(tmp_path):
    def fail(uri, write, limit):
        write(b"partial")
        raise OSError("offline")

    cache = MediaCache(tmp_path, 8, fail)
    with pytest.raises(MLError):
        with cache.acquire("s3://bucket/a"):
            pass
    assert not list(tmp_path.iterdir())
    cache.download = lambda uri, write, limit: write(b"ok")
    with cache.acquire("s3://bucket/a") as path:
        assert path.read_bytes() == b"ok"


@pytest.mark.parametrize(
    "uri",
    [
        "https://example/a",
        "s3://bucket/",
        "s3:///key",
        "s3://bucket/key?x=1",
        "s3://user@bucket/key",
    ],
)
def test_s3_only(uri):
    with pytest.raises(MLError):
        split_s3_uri(uri)


def test_cache_budget_includes_partial_downloads(tmp_path):
    entered = threading.Event()
    release = threading.Event()

    def download(uri, write, limit):
        write(b"123456")
        if uri.endswith("a"):
            entered.set()
            release.wait(2)

    cache = MediaCache(tmp_path, 8, download)

    def fetch():
        with cache.acquire("s3://bucket/a") as path:
            return path.read_bytes()

    with ThreadPoolExecutor(1) as pool:
        first = pool.submit(fetch)
        assert entered.wait(1)
        with pytest.raises(MLError) as error:
            with cache.acquire("s3://bucket/b"):
                pass
        assert error.value.code == "busy"
        assert sum(cache.staging.values()) == 6
        release.set()
        assert first.result() == b"123456"
    assert not cache.staging
