"""Frozen old-contract evidence and current-adapter regression; no services needed."""

import copy
import json
import sys
from itertools import pairwise
from pathlib import Path

import pytest

ROOT = Path(__file__).resolve().parents[3]
sys.path[:0] = [str(ROOT / "ml"), str(ROOT / "Eval/tools")]

from app.adapters.funasr import transcript_from_result
from diagnose_asr import violations

HERE = Path(__file__).parent
MINIMAL_CASES = json.loads((HERE / "minimal-repro.json").read_text())


@pytest.mark.parametrize("case", MINIMAL_CASES)
def test_two_real_tokens_reproduce_historical_non_overlap_violation(case):
    result = case["result"]
    duration = case["duration_ms"]
    invalid = violations(result, 0, duration)
    assert len(invalid) == 1
    assert invalid[0]["token_index"] == 1
    assert invalid[0]["overlap_ms"] == (60 if case["video_id"] == 3 else 140)
    assert not invalid[0]["zero_or_negative_duration"]


@pytest.mark.parametrize("offset", [0, 3000])
@pytest.mark.parametrize(
    "result,duration",
    [
        pytest.param(case["result"], case["duration_ms"], id=f"two-token-{case['video_id']}")
        for case in MINIMAL_CASES
    ]
    + [
        pytest.param(
            json.loads((HERE / f"video-{video_id}-raw.json").read_text())[0],
            duration,
            id=f"full-{video_id}",
        )
        for video_id, duration in [(3, 108000), (4, 96000)]
    ],
)
def test_frozen_outputs_preserve_recognition_and_allowed_overlap(result, duration, offset):
    before = copy.deepcopy(result)
    transcript = transcript_from_result(result, "zh", duration + offset, offset, duration + offset)

    assert transcript.language == "zh"
    assert transcript.duration_ms == duration + offset
    assert [sentence.text for sentence in transcript.sentences] == [
        source["text"] for source in result["sentence_info"]
    ]
    words = [word for sentence in transcript.sentences for word in sentence.words]
    assert [word.w for word in words] == result["raw_text"].split()
    assert [[word.s, word.e] for word in words] == [
        [start + offset, end + offset] for start, end in result["timestamp"]
    ]
    assert all(offset <= word.s < word.e <= duration + offset for word in words)
    assert all(left.s <= right.s for left, right in pairwise(words))
    assert any(left.e > right.s for left, right in pairwise(words))
    for sentence, source in zip(transcript.sentences, result["sentence_info"], strict=True):
        assert [[word.s, word.e] for word in sentence.words] == [
            [start + offset, end + offset] for start, end in source["timestamp"]
        ]
        assert sentence.start_ms == min(word.s for word in sentence.words)
        assert sentence.end_ms == max(word.e for word in sentence.words)
        assert offset <= sentence.start_ms < sentence.end_ms <= duration + offset
    assert result == before


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
