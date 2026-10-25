from fastapi import Request
from fastapi.responses import JSONResponse


class MLError(Exception):
    def __init__(self, status: int, code: str, message: str):
        super().__init__(message)
        self.status = status
        self.code = code
        self.message = message


def error_response(status: int, code: str, message: str) -> JSONResponse:
    return JSONResponse(
        status_code=status,
        content={"error": {"code": code, "message": message}},
        headers={"Retry-After": "2"} if status == 503 else None,
    )


async def handle_error(request: Request, exc: MLError) -> JSONResponse:
    return error_response(exc.status, exc.code, exc.message)


def deadline_exceeded() -> MLError:
    return MLError(504, "deadline_exceeded", "Request deadline exceeded")
