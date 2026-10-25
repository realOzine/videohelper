import copy

import numpy as np
import pytest
import yaml

from app.adapters.funasr import FunASRTranscriber, transcript_from_result
from app.api.errors import MLError
from app.api.schemas import TranscriberInfo
from app.core.fingerprint import fingerprint
from app.core.registry import Registry
from app.settings import Settings


def config():
    manifest = Settings.models_file.with_name("models.local.yaml")
    return yaml.safe_load(manifest.read_text())["models"][1]


def adapter():
    settings = config()
    info = TranscriberInfo(
        id=settings["id"],
        languages=settings["languages"],
        max_window_ms=settings["max_window_ms"],
        fingerprint=fingerprint(settings, fake=False),
    )
    return FunASRTranscriber(info, settings)


def result():
    return {
        "text": "欢迎使用。视频助手。",
        "raw_text": "欢 迎 使 用 视 频 助 手",
        "timestamp": [
            [0, 100],
            [100, 200],
            [200, 300],
            [300, 400],
            [600, 700],
            [700, 800],
            [800, 900],
            [900, 1000],
        ],
        "sentence_info": [
            {
                "text": "欢迎使用。",
                "raw_text": "欢迎使用",
                "timestamp": [[0, 100], [100, 200], [200, 300], [300, 400]],
            },
            {
                "text": "视频助手。",
                "raw_text": "视频助手",
                "timestamp": [[600, 700], [700, 800], [800, 900], [900, 1000]],
            },
        ],
    }


def test_sentence_text_and_all_tokens_preserved_at_absolute_offset():
    original = result()
    transcript = transcript_from_result(original, "zh", 10000, 3000, 4500)
    assert transcript.duration_ms == 10000
    assert [sentence.text for sentence in transcript.sentences] == ["欢迎使用。", "视频助手。"]
    assert [sentence.start_ms for sentence in transcript.sentences] == [3000, 3600]
    assert [word.w for sentence in transcript.sentences for word in sentence.words] == original[
        "raw_text"
    ].split()
    assert transcript.sentences[-1].words[-1].e == 4000
    assert original == result()


@pytest.mark.parametrize(
    "changes",
    [
        {1: [90, 200]},
        {3: [300, 850]},
        {0: [0, 1200]},
        {3: [600, 1000], 4: [600, 700]},
        {index: [0, 1000 - index * 100] for index in range(8)},
    ],
    ids=[
        "within_sentence",
        "across_sentences",
        "contained",
        "equal_word_starts",
        "equal_sentence_starts",
    ],
)
def test_overlapping_words_preserve_recognition_order_and_sentence_envelope(changes):
    original = result()
    for index, stamp in changes.items():
        original["timestamp"][index] = stamp
    for index, sentence in enumerate(original["sentence_info"]):
        sentence["timestamp"] = original["timestamp"][index * 4 : (index + 1) * 4]
    before = copy.deepcopy(original)

    transcript = transcript_from_result(original, "zh", 10000, 3000, 4500)

    assert [sentence.text for sentence in transcript.sentences] == ["欢迎使用。", "视频助手。"]
    words = [word for sentence in transcript.sentences for word in sentence.words]
    assert [word.w for word in words] == original["raw_text"].split()
    assert [[word.s, word.e] for word in words] == [
        [3000 + start, 3000 + end] for start, end in original["timestamp"]
    ]
    for sentence in transcript.sentences:
        assert sentence.start_ms == min(word.s for word in sentence.words)
        assert sentence.end_ms == max(word.e for word in sentence.words)
    assert original == before


def test_silence_and_boundary_clipping():
    assert (
        transcript_from_result({"text": "", "timestamp": []}, "zh", 10000, 3000, 4500).sentences
        == []
    )
    original = result()
    original["timestamp"][0][0] = -10
    original["sentence_info"][0]["timestamp"][0][0] = -10
    transcript = transcript_from_result(original, "zh", 10000, 3000, 3950)
    assert transcript.sentences[0].words[0].s == 3000
    assert transcript.sentences[-1].words[-1].e == 3950


@pytest.mark.parametrize(
    "fault",
    [
        "missing_token",
        "lost_sentence",
        "reverse_start",
        "reverse_across",
        "zero",
        "outside",
        "reorder",
    ],
)
def test_invalid_alignment_fails_instead_of_losing_recognition(fault):
    broken = copy.deepcopy(result())
    if fault == "missing_token":
        broken["raw_text"] = "欢 迎"
    elif fault == "lost_sentence":
        broken["sentence_info"].pop()
    elif fault == "reverse_start":
        broken["timestamp"][2][0] = 90
        broken["sentence_info"][0]["timestamp"][2][0] = 90
    elif fault == "reverse_across":
        broken["timestamp"][3] = [650, 750]
        broken["sentence_info"][0]["timestamp"][3] = [650, 750]
    elif fault == "zero":
        broken["timestamp"][1] = [100, 100]
        broken["sentence_info"][0]["timestamp"][1] = [100, 100]
    elif fault == "outside":
        broken["timestamp"][-1] = [1600, 1700]
        broken["sentence_info"][-1]["timestamp"][-1] = [1600, 1700]
    else:
        broken["sentence_info"].reverse()
    with pytest.raises(MLError) as error:
        transcript_from_result(broken, "zh", 10000, 3000, 4500)
    assert error.value.code == "internal"


def test_transcriber_decodes_window_and_asks_for_raw_sentence_timestamps(media_file, monkeypatch):
    instance = adapter()
    received = {}

    class Model:
        def generate(self, **kwargs):
            received.update(kwargs)
            return [{"text": "", "timestamp": []}]

    instance.model = Model()
    transcript = instance.transcribe(media_file, "zh", 500, 1000)
    assert transcript.sentences == []
    assert received["input"].shape == (8000,)
    assert np.isfinite(received["input"]).all()
    assert received["sentence_timestamp"] is received["return_raw_text"] is True
    assert received["pred_timestamp"] is True
    assert received["fs"] == 16000


def test_pins_and_fingerprint_include_all_three_models():
    instance = adapter()
    for component in ("vad_model", "punc_model"):
        changed = copy.deepcopy(instance.config)
        changed["preprocess"][component]["revision"] = "main"
        assert fingerprint(changed, fake=False) != instance.info.fingerprint
        with pytest.raises(ValueError, match="pinned"):
            FunASRTranscriber(instance.info, changed)
    changed = {**instance.config, "device": "mps"}
    with pytest.raises(ValueError, match="CPU"):
        FunASRTranscriber(instance.info, changed)


def test_overlap_adapter_version_changes_real_asr_fingerprint():
    current = config()
    assert current["preprocess"]["adapter_version"] == 2
    previous = copy.deepcopy(current)
    previous["preprocess"]["adapter_version"] = 1
    assert fingerprint(current, fake=False) != fingerprint(previous, fake=False)


def test_combined_registry_does_not_fall_back_to_fake(monkeypatch):
    from app.adapters.clap import CLAPEmbedder
    from app.adapters.wemm import WeMMEmbedder

    monkeypatch.setattr(CLAPEmbedder, "load", lambda self: None)
    monkeypatch.setattr(WeMMEmbedder, "load", lambda self: None)
    monkeypatch.setattr(FunASRTranscriber, "load", lambda self: None)
    registry = Registry(Settings.models_file.with_name("models.local.yaml"), fake=False)
    assert isinstance(registry.embedders["wemm-2b@r1"], WeMMEmbedder)
    assert isinstance(registry.transcribers["paraformer-zh@r1"], FunASRTranscriber)
    assert registry.spaces().transcribers[0].languages == ["zh"]


def test_incomplete_offline_snapshot_never_serves_uninitialized_weights(tmp_path, monkeypatch):
    import sys
    from types import SimpleNamespace

    instance = adapter()
    monkeypatch.setitem(
        sys.modules,
        "funasr",
        SimpleNamespace(
            AutoModel=lambda **kwargs: pytest.fail(
                "Model must not load from an incomplete snapshot"
            ),
        ),
    )
    monkeypatch.setitem(
        sys.modules,
        "huggingface_hub",
        SimpleNamespace(
            constants=SimpleNamespace(HF_HUB_OFFLINE=True),
            snapshot_download=lambda **kwargs: str(tmp_path),
        ),
    )
    with pytest.raises(RuntimeError, match="incomplete"):
        instance.load()
