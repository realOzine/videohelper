import asyncio
import time
from collections.abc import Callable
from typing import TypeVar

from app.api.errors import MLError, deadline_exceeded

T = TypeVar("T")


class Gate:
    def __init__(self, concurrency: int, queue_limit: int):
        self.slots = asyncio.Semaphore(concurrency)
        self.capacity = concurrency + queue_limit
        self.admitted = 0

    async def run(self, operation: Callable[[], T], deadline: float | None) -> T:
        if deadline is not None and time.monotonic() >= deadline:
            raise deadline_exceeded()
        if self.admitted >= self.capacity:
            raise MLError(503, "busy", "Model queue is full")
        self.admitted += 1
        started = False

        async def execute():
            nonlocal started
            async with self.slots:
                if deadline is not None and time.monotonic() >= deadline:
                    raise deadline_exceeded()
                started = True
                return await asyncio.to_thread(operation)

        task = asyncio.create_task(execute())

        def completed(done):
            self.admitted -= 1
            # Consume errors if the HTTP caller has timed out or disconnected.
            if not done.cancelled():
                done.exception()

        task.add_done_callback(completed)
        try:
            remaining = None if deadline is None else max(0, deadline - time.monotonic())
            return await asyncio.wait_for(asyncio.shield(task), remaining)
        except (TimeoutError, asyncio.CancelledError) as exc:
            # Python cannot cancel an executing thread; keep its slot until it exits.
            if not started:
                task.cancel()
            if isinstance(exc, TimeoutError):
                raise deadline_exceeded() from exc
            raise
