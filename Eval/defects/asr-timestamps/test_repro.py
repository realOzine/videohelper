"""Frozen real-output diagnosis; no model or running services are needed."""

import json
import sys
from pathlib import Path

import pytest

ROOT = Path(__file__).resolve().parents[3]
sys.path[:0] = [str(ROOT / "ml"), str(ROOT / "Eval/tools")]

from app.adapters.funasr import transcript_from_result
from app.api.errors import MLError
from diagnose_asr import violations

HERE = Path(__file__).parent


@pytest.mark.parametrize("case", json.loads((HERE / "minimal-repro.json").read_text()))
def test_two_real_tokens_reproduce_adapter_rejection(case):
    result = case["result"]
    duration = case["duration_ms"]
    with pytest.raises(MLError, match="ASR word timestamps are invalid"):
        transcript_from_result(result, "zh", duration, 0, duration)
    invalid = violations(result, 0, duration)
    assert len(invalid) == 1
    assert invalid[0]["token_index"] == 1
    assert invalid[0]["overlap_ms"] == (60 if case["video_id"] == 3 else 140)
    assert not invalid[0]["zero_or_negative_duration"]


@pytest.mark.parametrize("video_id,expected_index,token_count", [(3, 106, 146), (4, 225, 240)])
def test_full_output_overlap_originates_at_adjacent_vad_segments(video_id, expected_index, token_count):
    result = json.loads((HERE / f"video-{video_id}-raw.json").read_text())[0]
    stages = json.loads((HERE / f"video-{video_id}-stages.json").read_text())
    vad = stages[0]["results"][0]["value"]
    asr = [r for stage in stages if stage["stage"] == "asr" for r in stage["results"]]
    restored = [None] * len(vad)
    sorted_segments = sorted(enumerate(vad), key=lambda item: item[1][1] - item[1][0])
    for (index, segment), value in zip(sorted_segments, asr, strict=True):
        restored[index] = value
    flattened = []
    segment_ids = []
    for index, (segment, value) in enumerate(zip(vad, restored, strict=True)):
        flattened.extend([[start + segment[0], end + segment[0]] for start, end in value["timestamp"]])
        segment_ids.extend([index] * len(value["timestamp"]))
    assert flattened == result["timestamp"]
    assert len(flattened) == token_count
    assert [stamp for sentence in result["sentence_info"] for stamp in sentence["timestamp"]] == flattened
    invalid = violations(result, 0, 108000 if video_id == 3 else 96000)
    assert [item["token_index"] for item in invalid] == [expected_index]
    previous_segment, current_segment = segment_ids[expected_index - 1:expected_index + 1]
    assert current_segment == previous_segment + 1
    assert vad[previous_segment][1] == vad[current_segment][0]
