import hmac

from fastapi import Request

from app.api.errors import MLError


async def authenticate(request: Request):
    token = request.headers.get("X-Internal-Token", "")
    expected = request.app.state.settings.token
    if not hmac.compare_digest(token.encode(), expected.encode()):
        raise MLError(401, "unauthorized", "Invalid internal token")
