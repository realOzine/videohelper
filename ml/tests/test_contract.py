import jsonschema
import pytest
from test_routes import QUERY, URI


def validate(contract, schema, value):
    jsonschema.Draft202012Validator({**schema, "components": contract["components"]}).validate(
        value
    )


@pytest.mark.parametrize(
    "method,path,body",
    [
        ("get", "/healthz", None),
        ("get", "/readyz", None),
        ("get", "/v1/spaces", None),
        ("post", "/v1/embed", QUERY),
        (
            "post",
            "/v1/transcribe",
            {
                "model": "paraformer-zh@r1",
                "uri": URI,
                "language": "zh",
                "start_ms": 500,
                "end_ms": 1500,
            },
        ),
    ],
)
def test_success_matches_contract(client, contract, method, path, body):
    operation = contract["paths"][path][method]
    if body is not None:
        validate(contract, operation["requestBody"]["content"]["application/json"]["schema"], body)
    response = client.request(method, path, json=body)
    assert response.status_code == 200, response.text
    schema = operation["responses"]["200"]["content"]["application/json"]["schema"]
    validate(contract, schema, response.json())


def test_error_and_exactly_one_input_contract(client, contract):
    response = client.post("/v1/embed", json={**QUERY, "inputs": []})
    schema = contract["paths"]["/v1/embed"]["post"]["responses"]["400"]["content"][
        "application/json"
    ]["schema"]
    validate(contract, schema, response.json())
    for body in ({}, {"text": None}, {"text": "x", "audio": None}):
        with pytest.raises(jsonschema.ValidationError):
            validate(contract, {"$ref": "#/components/schemas/EmbedInput"}, body)
