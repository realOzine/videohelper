import hashlib
import json


def fingerprint(config: dict, *, fake: bool) -> str:
    payload = {
        key: config.get(key)
        for key in (
            "adapter",
            "weights",
            "revision",
            "weights_digest",
            "dtype",
            "dim",
            "metric",
            "preprocess",
        )
    }
    # Fake output must never share a fingerprint with real weights.
    if fake:
        payload["adapter"] = "fake-v1:" + config["adapter"]
        payload["output"] = {
            key: config.get(key)
            for key in (
                "dim",
                "metric",
                "query_inputs",
                "doc_inputs",
                "languages",
            )
        }
    encoded = json.dumps(payload, sort_keys=True, separators=(",", ":"), ensure_ascii=False)
    return "sha256:" + hashlib.sha256(encoded.encode()).hexdigest()
