from pathlib import Path

import numpy as np

from app.adapters.base import ImageInput, MediaInput, TextInput
from app.api.errors import MLError
from app.api.schemas import Role, SpaceInfo
from app.media.audio import load

MODEL_ID = "laion/clap-htsat-unfused"
REVISION = "8fa0f1c6d0433df6e97c127f64b2a1d6c0dcda8a"
SAMPLE_RATE = 48000
MAX_SAMPLES = 480000


class CLAPEmbedder:
    kind = "embedder"
    shared_query_gate = False

    def __init__(self, info: SpaceInfo, config: dict):
        self.info = info
        self.config = config
        if (config["weights"], config["revision"]) != (MODEL_ID, REVISION):
            raise ValueError("CLAP requires the pinned official HTSAT unfused model")
        expected = {
            "sample_rate": 48000,
            "max_samples": 480000,
            "padding": "repeatpad",
            "truncation": "rand_trunc",
            "max_text_tokens": 512,
        }
        if any(config["preprocess"].get(key) != value for key, value in expected.items()):
            raise ValueError("CLAP preprocessing must match the pinned 48kHz/10s configuration")
        if config["device"] != "cpu" or config["dtype"] != "float32":
            raise ValueError("Local CLAP uses CPU float32")
        if (info.dim, info.metric, info.query_inputs, info.doc_inputs, info.max_span_ms) != (
            512,
            "cosine",
            ["text"],
            ["audio"],
            10000,
        ):
            raise ValueError("CLAP requires 512/cosine, text queries and audio documents up to 10s")

    @staticmethod
    def prepare(config: dict) -> Path:
        if (config["weights"], config["revision"]) != (MODEL_ID, REVISION):
            raise ValueError("CLAP requires the pinned official HTSAT unfused model")
        try:
            from huggingface_hub import constants, snapshot_download
        except ImportError as exc:
            raise RuntimeError("Install videohelper-ml-clap[inference]") from exc

        directory = Path(
            snapshot_download(
                MODEL_ID,
                revision=REVISION,
                local_files_only=constants.HF_HUB_OFFLINE,
                allow_patterns=["*.json", "merges.txt", "pytorch_model.bin"],
            )
        )
        required = (
            "config.json",
            "preprocessor_config.json",
            "pytorch_model.bin",
            "tokenizer.json",
            "tokenizer_config.json",
            "special_tokens_map.json",
            "merges.txt",
            "vocab.json",
        )
        missing = [name for name in required if not (directory / name).is_file()]
        if missing:
            raise RuntimeError(f"Pinned CLAP snapshot is incomplete: {', '.join(missing)}")
        return directory

    def load(self):
        try:
            import torch
            from transformers import ClapModel, ClapProcessor
        except ImportError as exc:
            raise RuntimeError(
                "Install videohelper-ml-clap[inference] (or uv sync --extra clap)"
            ) from exc

        directory = self.prepare(self.config)
        self.processor = ClapProcessor.from_pretrained(
            directory,
            trust_remote_code=False,
            local_files_only=True,
        )
        self.model = (
            ClapModel.from_pretrained(
                directory,
                trust_remote_code=False,
                local_files_only=True,
                use_safetensors=False,
                weights_only=True,
                dtype=torch.float32,
            )
            .to("cpu")
            .eval()
        )
        if self.model.config.projection_dim != 512 or self.model.config.audio_config.enable_fusion:
            raise ValueError("CLAP checkpoint is not the expected unfused 512-dimensional model")

    def _audio_inputs(self, item: MediaInput):
        if item.end_ms - item.start_ms > 10000:
            raise MLError(400, "invalid_input", "CLAP audio window exceeds 10 seconds")
        samples = load(item.path, item.start_ms, item.end_ms, SAMPLE_RATE)
        if not 0 < len(samples) <= MAX_SAMPLES:
            raise MLError(400, "invalid_input", "CLAP audio exceeds the sample budget")
        # The length guard keeps rand_trunc on its deterministic, non-cropping branch.
        return self.processor.feature_extractor(
            samples,
            sampling_rate=SAMPLE_RATE,
            return_tensors="pt",
            max_length=MAX_SAMPLES,
            padding="repeatpad",
            truncation="rand_trunc",
        )

    def embed(self, inputs: list[TextInput | MediaInput | ImageInput], role: Role) -> np.ndarray:
        import torch
        import torch.nn.functional as functional

        vectors = []
        with torch.inference_mode():
            for item in inputs:
                if isinstance(item, TextInput):
                    tokens = self.processor.tokenizer(
                        item.text, return_tensors="pt", truncation=False
                    )
                    if tokens["input_ids"].shape[1] > 512:
                        raise MLError(400, "invalid_input", "CLAP text exceeds 512 tokens")
                    output = self.model.get_text_features(**tokens, return_dict=True)
                elif isinstance(item, MediaInput) and item.kind == "audio":
                    output = self.model.get_audio_features(
                        **self._audio_inputs(item), return_dict=True
                    )
                else:
                    raise MLError(400, "invalid_input", "CLAP accepts text or audio inputs")
                vectors.append(
                    functional.normalize(output.pooler_output.float(), dim=-1).numpy()[0]
                )
        return np.stack(vectors).astype(np.float32, copy=False)
