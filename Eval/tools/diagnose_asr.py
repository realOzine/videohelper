"""Offline timestamp diagnostics for frozen historical ASR fixtures.

The former PostgreSQL/S3 capture command lives in the server Git history.
"""

def violations(result, start_ms, end_ms):
    """Mirror adapter clipping while retaining every offending raw timestamp."""
    tokens = result["raw_text"].split()
    stamps = result["timestamp"]
    if len(tokens) != len(stamps):
        raise ValueError("token/timestamp count mismatch")
    invalid = []
    previous = start_ms
    for index, (token, (raw_start, raw_end)) in enumerate(zip(tokens, stamps, strict=True)):
        start = min(end_ms, max(start_ms, start_ms + int(raw_start)))
        end = min(end_ms, max(start_ms, start_ms + int(raw_end)))
        if not previous <= start < end:
            invalid.append({
                "token_index": index, "token": token,
                "raw_timestamp": [raw_start, raw_end],
                "clipped_timestamp": [start, end], "previous_end_ms": previous,
                "overlap_ms": max(0, previous - start),
                "zero_or_negative_duration": start >= end,
            })
        previous = end
    return invalid

