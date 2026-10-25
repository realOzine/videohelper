import asyncio
import threading
import time

import pytest

from app.api.errors import MLError
from app.core.gate import Gate


def test_bounded_queue_deadline_and_running_slot_is_held():
    async def scenario():
        gate = Gate(1, 1)
        entered = threading.Event()
        release = threading.Event()
        calls = []

        def block():
            entered.set()
            release.wait(2)
            return "done"

        first = asyncio.create_task(gate.run(block, time.monotonic() + 0.05))
        await asyncio.to_thread(entered.wait, 1)
        second = asyncio.create_task(
            gate.run(lambda: calls.append("queued"), time.monotonic() + 0.1)
        )
        await asyncio.sleep(0)
        with pytest.raises(MLError) as busy:
            await gate.run(lambda: None, None)
        assert busy.value.code == "busy"
        with pytest.raises(MLError) as expired:
            await first
        assert expired.value.code == "deadline_exceeded"
        assert gate.admitted == 2
        with pytest.raises(MLError):
            await second
        assert calls == []
        release.set()
        for _ in range(100):
            if gate.admitted == 0:
                break
            await asyncio.sleep(0.005)
        assert gate.admitted == 0
        assert await gate.run(lambda: "new", None) == "new"

    asyncio.run(scenario())


def test_expired_request_never_starts_and_cancellation_releases_queue():
    async def scenario():
        gate = Gate(1, 1)
        with pytest.raises(MLError):
            await gate.run(lambda: pytest.fail("called"), time.monotonic() - 1)
        task = asyncio.create_task(gate.run(lambda: "ok", None))
        task.cancel()
        with pytest.raises(asyncio.CancelledError):
            await task
        assert gate.admitted == 0

    asyncio.run(scenario())
