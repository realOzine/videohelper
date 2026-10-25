#!/usr/bin/env python3
"""Serve only Eval on localhost, with HTTP Range support for video seeking."""
from pathlib import Path

import uvicorn
from starlette.applications import Starlette
from starlette.routing import Mount
from starlette.staticfiles import StaticFiles

if __name__ == "__main__":
    root = Path(__file__).resolve().parents[1]
    print("Review: http://127.0.0.1:18766/review/index.html", flush=True)
    app = Starlette(routes=[Mount("/", app=StaticFiles(directory=root))])
    uvicorn.run(app, host="127.0.0.1", port=18766)
