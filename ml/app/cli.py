import argparse
import json
import os
from pathlib import Path

import yaml

from app.core.plugins import load_adapter
from app.core.registry import Manifest


def main():
    parser = argparse.ArgumentParser(description="VideoHelper ML foreground service")
    parser.add_argument("command", choices=("serve", "prepare", "describe"))
    parser.add_argument("--models", type=Path, required=True)
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=8000)
    args = parser.parse_args()
    manifest = Manifest.model_validate(yaml.safe_load(args.models.read_text()))
    ids = [config.id for config in manifest.models]
    if len(ids) != len(set(ids)):
        parser.error("Duplicate model id")
    if args.command == "serve":
        import uvicorn

        os.environ["ML_MODELS_FILE"] = str(args.models.resolve())
        uvicorn.run("app.main:app", host=args.host, port=args.port)
    elif args.command == "prepare":
        for config in manifest.models:
            adapter = load_adapter(config.adapter, config.kind)
            adapter.prepare(config.model_dump())
        print("Pinned model weights are prepared")
    else:
        print(json.dumps(manifest.model_dump(), indent=2))


if __name__ == "__main__":
    main()
