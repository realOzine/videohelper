import hashlib
from pathlib import Path

import numpy as np

from app.adapters.base import ImageInput, MediaInput, TextInput
from app.api.errors import MLError
from app.api.schemas import Role, SpaceInfo
from app.media.image import load_image
from app.media.video import sample_frames

MODEL_ID = "tencent/WeMM-Embedding-2B"
REVISION = "bbd6cd4bf52cfc6716f752a2df80b2706720bd95"
# Reviewed upstream Qwen3.5 subclass: final-token pooling + L2 normalization.
REMOTE_CODE_SHA256 = "ac255e1fad459cc3e68891d6c3327f4486922aed02fb3c5c13fb53277ba8e94f"


class WeMMEmbedder:
    kind = "embedder"
    shared_query_gate = True

    def __init__(self, info: SpaceInfo, config: dict):
        self.info = info
        self.config = config
        self.preprocess = config["preprocess"]
        if config["weights"] != MODEL_ID or config["revision"] != REVISION:
            raise ValueError("WeMM requires the reviewed, pinned official model revision")
        if info.query_inputs != ["text"] or set(info.doc_inputs) != {"text", "video", "image"}:
            raise ValueError("WeMM supports text queries and text/video/image documents")
        if config["max_concurrency"] != 1:
            raise ValueError("WeMM has mutable position state and requires max_concurrency=1")

    @staticmethod
    def prepare(config: dict) -> Path:
        if config["weights"] != MODEL_ID or config["revision"] != REVISION:
            raise ValueError("WeMM requires the reviewed, pinned official model revision")
        try:
            from huggingface_hub import constants, snapshot_download
        except ImportError as exc:
            raise RuntimeError("Install videohelper-ml-wemm[inference]") from exc

        directory = Path(
            snapshot_download(
                repo_id=MODEL_ID,
                revision=REVISION,
                local_files_only=constants.HF_HUB_OFFLINE,
                allow_patterns=[
                    "*.json",
                    "*.jinja",
                    "model.safetensors",
                    "modeling_wemm_embedding.py",
                ],
            )
        )
        code = directory / "modeling_wemm_embedding.py"
        if hashlib.sha256(code.read_bytes()).hexdigest() != REMOTE_CODE_SHA256:
            raise RuntimeError("Pinned WeMM remote code does not match the reviewed source")
        return directory

    def load(self):
        try:
            import torch
            from transformers import AutoModel, AutoProcessor
        except ImportError as exc:
            raise RuntimeError(
                "Install videohelper-ml-wemm[inference] (or uv sync --extra wemm)"
            ) from exc

        if self.config["device"] not in ("cpu", "mps"):
            raise ValueError("Local WeMM supports device cpu or mps")
        self.device = self.config["device"]
        if self.device == "mps" and not torch.backends.mps.is_available():
            raise ValueError("MPS is unavailable; set device=cpu and dtype=float32")
        dtype = {"float16": torch.float16, "bfloat16": torch.bfloat16, "float32": torch.float32}
        if self.config["dtype"] not in dtype:
            raise ValueError("Unsupported WeMM dtype")
        directory = self.prepare(self.config)
        self.processor = AutoProcessor.from_pretrained(
            directory,
            trust_remote_code=False,
            use_fast=self.preprocess["use_fast"],
        )
        self.model = (
            AutoModel.from_pretrained(
                directory,
                trust_remote_code=True,
                dtype=dtype[self.config["dtype"]],
                attn_implementation="eager",
                local_files_only=True,
            )
            .to(self.device)
            .eval()
        )
        if self.info.dim not in self.model.config.matryoshka_dimensions:
            raise ValueError("Unsupported WeMM Matryoshka dimension")

    def _inputs(self, item: TextInput | MediaInput | ImageInput):
        kwargs = {}
        if isinstance(item, TextInput):
            content = {"type": "text", "text": item.text}
        elif isinstance(item, ImageInput):
            content = {"type": "image"}
            kwargs["images"] = [load_image(item.path)]
            kwargs["images_kwargs"] = {
                "size": {
                    "shortest_edge": self.preprocess["image_min_pixels"],
                    "longest_edge": self.preprocess["image_max_pixels"],
                }
            }
        else:
            if item.kind != "video":
                raise MLError(400, "invalid_input", "WeMM does not support audio")
            count = self.preprocess["frames_per_clip"]
            frames = sample_frames(item.path, item.start_ms, item.end_ms, count, None)
            content = {"type": "video"}
            kwargs["videos"] = [frames]
            kwargs["videos_kwargs"] = {
                "do_sample_frames": False,
                "video_metadata": [
                    {
                        "total_num_frames": count,
                        "fps": count * 1000 / (item.end_ms - item.start_ms),
                        "frames_indices": list(range(count)),
                    }
                ],
                "size": {
                    "shortest_edge": self.preprocess["video_min_pixels"],
                    "longest_edge": self.preprocess["video_max_pixels"],
                },
            }
        messages = [{"role": "user", "content": [content]}]
        prompt = self.processor.apply_chat_template(
            messages,
            tokenize=False,
            add_generation_prompt=False,
        )
        inputs = self.processor(text=prompt, return_tensors="pt", **kwargs)
        if inputs["input_ids"].shape[1] > self.preprocess["max_tokens"]:
            raise MLError(400, "invalid_input", "Input exceeds WeMM token budget")
        return inputs.to(self.device)

    def embed(self, inputs: list[TextInput | MediaInput | ImageInput], role: Role) -> np.ndarray:
        import torch
        import torch.nn.functional as functional

        vectors = []
        # shortcut: encode one input at a time to bound Mac memory; batch after profiling.
        with torch.inference_mode():
            for item in inputs:
                embedding = self.model.embedding(**self._inputs(item), use_cache=False).float()
                embedding = functional.normalize(embedding[..., : self.info.dim], dim=-1)
                vectors.append(embedding.cpu().numpy()[0])
        return np.stack(vectors).astype(np.float32, copy=False)
