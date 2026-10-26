from pathlib import Path

from app.api.errors import MLError
from app.api.schemas import Sentence, TranscriberInfo, Transcript, Word
from app.media.audio import load
from app.media.probe import duration_ms

ASR_MODEL = ("funasr/paraformer-zh", "d7811ee3ac581fbcfdeb37c98c6ba674028433dc")
VAD_MODEL = ("funasr/fsmn-vad", "df20e6b30c653645fa4ff125cacfcabd1020a669")
PUNC_MODEL = ("funasr/ct-punc", "d0e55e2b8722a78b63705ff443d09c4f86e5d750")


def transcript_from_result(
    result: dict,
    language: str,
    duration: int,
    start_ms: int,
    end_ms: int,
) -> Transcript:
    if not result.get("text"):
        return Transcript(language=language, duration_ms=duration, sentences=[])
    tokens = result.get("raw_text", "").split()
    timestamps = result.get("timestamp", [])
    if not tokens or len(tokens) != len(timestamps):
        raise MLError(500, "internal", "ASR token timestamps are incomplete")
    sentences = []
    cursor = 0
    previous_start = start_ms
    for source in result.get("sentence_info", []):
        stamps = source["timestamp"]
        count = len(stamps)
        if not count or stamps != timestamps[cursor : cursor + count]:
            raise MLError(500, "internal", "ASR sentence timestamps do not align")
        words = []
        for token, (start, end) in zip(tokens[cursor : cursor + count], stamps, strict=True):
            # Frame-based ASR boundaries may extend slightly beyond the decoded window.
            start = min(end_ms, max(start_ms, start_ms + int(start)))
            end = min(end_ms, max(start_ms, start_ms + int(end)))
            if not previous_start <= start < end:
                raise MLError(500, "internal", "ASR word timestamps are invalid")
            words.append(Word(w=token, s=start, e=end))
            previous_start = start
        sentences.append(
            Sentence(
                start_ms=min(word.s for word in words),
                end_ms=max(word.e for word in words),
                text=source["text"],
                words=words,
            )
        )
        cursor += count
    if cursor != len(tokens):
        raise MLError(500, "internal", "ASR sentence segmentation lost recognized tokens")
    return Transcript(language=language, duration_ms=duration, sentences=sentences)


def _validate_pins(config: dict):
    pinned = [
        (config["weights"], config["revision"]),
        tuple(config["preprocess"]["vad_model"][key] for key in ("weights", "revision")),
        tuple(config["preprocess"]["punc_model"][key] for key in ("weights", "revision")),
    ]
    if pinned != [ASR_MODEL, VAD_MODEL, PUNC_MODEL]:
        raise ValueError("FunASR requires the reviewed pinned ASR/VAD/punctuation models")


class FunASRTranscriber:
    kind = "transcriber"
    shared_query_gate = False

    def __init__(self, info: TranscriberInfo, config: dict):
        self.info = info
        self.config = config
        self.preprocess = config["preprocess"]
        _validate_pins(config)
        if config["device"] != "cpu" or config["dtype"] != "float32":
            raise ValueError("Local FunASR uses CPU float32")
        if config["max_concurrency"] != 1:
            raise ValueError("FunASR requires max_concurrency=1 for mutable runtime state")

    @staticmethod
    def prepare(config: dict) -> list[str]:
        _validate_pins(config)
        try:
            from huggingface_hub import constants, snapshot_download
        except ImportError as exc:
            raise RuntimeError("Install videohelper-ml-funasr[inference]") from exc

        directories = [
            snapshot_download(
                repo_id=repo,
                revision=revision,
                local_files_only=constants.HF_HUB_OFFLINE,
                allow_patterns=[
                    "model.pt",
                    "config.yaml",
                    "configuration.json",
                    "am.mvn",
                    "tokens.json",
                    "seg_dict",
                ],
            )
            for repo, revision in (ASR_MODEL, VAD_MODEL, PUNC_MODEL)
        ]
        required_files = [
            ("model.pt", "config.yaml", "configuration.json", "am.mvn", "tokens.json", "seg_dict"),
            ("model.pt", "config.yaml", "configuration.json", "am.mvn"),
            ("model.pt", "config.yaml", "configuration.json", "tokens.json"),
        ]
        for directory, required in zip(directories, required_files, strict=True):
            if any(not (Path(directory) / name).is_file() for name in required):
                # FunASR otherwise skips absent init_param and can serve random weights.
                raise RuntimeError("Pinned FunASR model snapshot is incomplete")
        return directories

    def load(self):
        try:
            from funasr import AutoModel
        except ImportError as exc:
            raise RuntimeError(
                "Install videohelper-ml-funasr[inference] (or uv sync --extra asr)"
            ) from exc

        directories = self.prepare(self.config)
        self.model = AutoModel(
            model=directories[0],
            model_revision=ASR_MODEL[1],
            vad_model=directories[1],
            vad_model_revision=VAD_MODEL[1],
            punc_model=directories[2],
            punc_model_revision=PUNC_MODEL[1],
            hub="hf",
            device="cpu",
            ncpu=self.preprocess["ncpu"],
            disable_update=True,
            disable_pbar=True,
            log_level="ERROR",
            trust_remote_code=False,
            vad_kwargs={
                "max_single_segment_time": self.preprocess["max_single_segment_time"],
                "trust_remote_code": False,
            },
            punc_kwargs={"trust_remote_code": False},
        )

    def transcribe(self, path: Path, language: str, start_ms: int, end_ms: int) -> Transcript:
        duration = duration_ms(path)
        end_ms = min(end_ms, duration)
        samples = load(path, start_ms, end_ms, 16000)
        results = self.model.generate(
            input=samples,
            fs=16000,
            batch_size_s=self.preprocess["batch_size_s"],
            batch_size_threshold_s=self.preprocess["batch_size_threshold_s"],
            pred_timestamp=True,
            sentence_timestamp=True,
            return_raw_text=True,
        )
        if len(results) != 1:
            raise MLError(500, "internal", "ASR returned an unexpected recording count")
        return transcript_from_result(results[0], language, duration, start_ms, end_ms)
