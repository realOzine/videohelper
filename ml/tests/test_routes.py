import numpy as np
import pytest

QUERY = {"space": "wemm-2b@r1", "role": "query", "inputs": [{"text": "hello"}]}
URI = "s3://test/normalized_video.mp4"


def test_auth_request_id_health_and_ready(client):
    assert client.get("/healthz", headers={"X-Internal-Token": "wrong"}).status_code == 200
    assert client.get("/readyz").status_code == 200
    response = client.get("/v1/spaces", headers={"X-Internal-Token": "wrong"})
    assert response.status_code == 401
    assert response.json()["error"]["code"] == "unauthorized"
    assert response.headers["X-Request-ID"] == "test-request"
    assert client.get("/v1/spaces", headers={"X-Request-ID": ""}).status_code == 400
    client.app.state.ready = False
    response = client.get("/readyz")
    assert response.status_code == 503
    assert response.headers["Retry-After"] == "2"
    assert client.post("/v1/embed", json=QUERY).json()["error"]["code"] == "not_ready"


def test_fake_embedding_deterministic_normalized_and_role_aware(client):
    first = client.post("/v1/embed", json=QUERY)
    assert first.status_code == 200, first.text
    assert first.json() == client.post("/v1/embed", json=QUERY).json()
    vector = first.json()["vectors"][0]
    assert len(vector) == first.json()["dim"] == 1024
    assert np.linalg.norm(vector) == pytest.approx(1)
    document = client.post("/v1/embed", json={**QUERY, "role": "document"})
    assert document.json()["vectors"] != first.json()["vectors"]


@pytest.mark.parametrize(
    "change",
    [
        {"inputs": []},
        {"inputs": [{}]},
        {"inputs": [{"text": None}]},
        {"inputs": [{"text": "x", "audio": None}]},
        {"inputs": [{"text": "x", "video": {"uri": URI, "start_ms": 0, "end_ms": 100}}]},
        {"inputs": [{"text": 123}]},
        {"inputs": [{"text": ""}]},
        {"inputs": [{"text": "x"}] * 65},
        {"role": "invalid"},
        {"extra": True},
        {
            "role": "document",
            "inputs": [{"video": {"uri": "https://example.com", "start_ms": 0, "end_ms": 100}}],
        },
        {"role": "document", "inputs": [{"video": {"uri": URI, "start_ms": 100, "end_ms": 100}}]},
        {"role": "document", "inputs": [{"video": {"uri": URI, "start_ms": -1, "end_ms": 100}}]},
        {"role": "document", "inputs": [{"video": {"uri": URI, "start_ms": False, "end_ms": 100}}]},
        {"role": "document", "inputs": [{"video": {"uri": URI, "start_ms": 0, "end_ms": 30001}}]},
        {"inputs": [{"video": {"uri": URI, "start_ms": 0, "end_ms": 100}}]},
    ],
)
def test_invalid_inputs(client, change):
    response = client.post("/v1/embed", json={**QUERY, **change})
    assert response.status_code == 400, response.text
    assert response.json()["error"]["code"] == "invalid_input"


def test_unknown_models_and_bad_timeout(client):
    response = client.post("/v1/embed", json={**QUERY, "space": "missing"})
    assert response.status_code == 404
    assert response.json()["error"]["code"] == "unknown_space"
    body = {"model": "missing", "uri": URI, "language": "zh", "start_ms": 0, "end_ms": 100}
    response = client.post("/v1/transcribe", json=body)
    assert response.status_code == 404
    assert response.json()["error"]["code"] == "unknown_model"
    for value in ("0", "-1", "1.5", "nan", ""):
        assert (
            client.post("/v1/embed", json=QUERY, headers={"X-Timeout-Ms": value}).status_code == 400
        )


def test_media_embed_clamps_end_and_checks_shapes(client, monkeypatch):
    body = {
        **QUERY,
        "role": "document",
        "inputs": [
            {"video": {"uri": URI, "start_ms": 1900, "end_ms": 2500}},
            {"text": "other"},
        ],
    }
    response = client.post("/v1/embed", json=body)
    assert response.status_code == 200, response.text
    assert len(response.json()["vectors"]) == 2
    adapter = client.app.state.registry.embedders[QUERY["space"]]
    for output in (np.ones((1, 1)), np.full((1, 1024), np.nan), np.zeros((1, 1024))):
        monkeypatch.setattr(adapter, "embed", lambda *args, value=output: value)
        response = client.post("/v1/embed", json=QUERY)
        assert response.status_code == 500
        assert response.json()["error"]["code"] == "internal"


def test_transcribe_window_absolute_and_validation(client):
    body = {
        "model": "paraformer-zh@r1",
        "uri": URI,
        "language": "zh",
        "start_ms": 900,
        "end_ms": 3000,
    }
    response = client.post("/v1/transcribe", json=body)
    assert response.status_code == 200, response.text
    transcript = response.json()
    assert transcript["duration_ms"] == 2000
    assert transcript["sentences"][0]["start_ms"] == 900
    assert transcript["sentences"][0]["end_ms"] <= 2000
    for change in ({"language": "en"}, {"end_ms": 901000}, {"start_ms": 2000}):
        response = client.post("/v1/transcribe", json={**body, **change})
        assert response.status_code == (422 if change == {"start_ms": 2000} else 400)


def test_media_failure_does_not_expose_source_error(client):
    def fail(*args):
        raise OSError("secret credentials")

    client.app.state.cache.download = fail
    response = client.post(
        "/v1/embed",
        json={
            **QUERY,
            "role": "document",
            "inputs": [
                {"video": {"uri": URI, "start_ms": 0, "end_ms": 100}},
            ],
        },
    )
    assert response.status_code == 422
    assert "secret" not in response.text


def test_http_busy_and_deadline_preserve_running_capacity(client, monkeypatch):
    import threading
    from concurrent.futures import ThreadPoolExecutor

    from app.core.gate import Gate

    entered = threading.Event()
    release = threading.Event()
    adapter = client.app.state.registry.embedders[QUERY["space"]]
    original = adapter.embed

    def slow(*args):
        entered.set()
        release.wait(2)
        return original(*args)

    monkeypatch.setattr(adapter, "embed", slow)
    client.app.state.registry.gates[QUERY["space"], "query"] = Gate(1, 0)
    with ThreadPoolExecutor(1) as pool:
        future = pool.submit(client.post, "/v1/embed", json=QUERY, headers={"X-Timeout-Ms": "100"})
        assert entered.wait(1)
        busy = client.post("/v1/embed", json=QUERY)
        assert busy.status_code == 503
        assert busy.headers["Retry-After"] == "2"
        expired = future.result()
        assert expired.status_code == 504
        assert expired.json()["error"]["code"] == "deadline_exceeded"
        assert client.post("/v1/embed", json=QUERY).status_code == 503
        release.set()
