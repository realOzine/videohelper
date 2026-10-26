from importlib.metadata import EntryPoint, entry_points
from typing import Any

ADAPTER_GROUP = "videohelper.ml.adapters.v1"


def _entry_point(name: str) -> EntryPoint:
    matches = list(entry_points(group=ADAPTER_GROUP, name=name))
    if not matches:
        raise ValueError(
            f"Real adapter {name!r} is not installed ({ADAPTER_GROUP}). "
            "Install its plugin package or sync the selected locked profile."
        )
    if len(matches) != 1:
        raise ValueError(f"Duplicate providers for adapter {name!r} ({ADAPTER_GROUP})")
    return matches[0]


def load_adapter(name: str, kind: str) -> Any:
    entry = _entry_point(name)
    try:
        adapter = entry.load()
    except ImportError as exc:
        raise ValueError(f"Cannot import real adapter {name!r}: {exc}") from exc
    if (
        not callable(adapter)
        or getattr(adapter, "kind", None) != kind
        or not callable(getattr(adapter, "load", None))
        or not callable(getattr(adapter, "prepare", None))
        or not callable(getattr(adapter, "embed" if kind == "embedder" else "transcribe", None))
        or not isinstance(getattr(adapter, "shared_query_gate", None), bool)
    ):
        raise ValueError(f"Incompatible provider for adapter {name!r}, expected {kind}")
    return adapter


def adapter_distribution(name: str) -> dict[str, str]:
    distribution = _entry_point(name).dist
    if distribution is None:
        raise ValueError(f"Adapter {name!r} has no installed distribution metadata")
    return {"name": distribution.name, "version": distribution.version}
