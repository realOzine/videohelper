import platform
from importlib.metadata import PackageNotFoundError, version

from app.api.schemas import RuntimeModel, RuntimeResponse
from app.core.plugins import adapter_distribution


def describe_runtime(registry, *, fake: bool) -> RuntimeResponse:
    try:
        service_version = version("videohelper-ml")
    except PackageNotFoundError:
        service_version = "uninstalled"
    packages = {}
    for name in ("torch", "torchvision", "torchaudio", "transformers", "funasr", "numpy", "av"):
        try:
            packages[name] = version(name)
        except PackageNotFoundError:
            pass
    spaces = registry.spaces()
    infos = {info.id: info for info in spaces.spaces + spaces.transcribers}
    models = []
    for config in registry.configs.values():
        distribution = None if fake else adapter_distribution(config.adapter)
        models.append(
            RuntimeModel(
                id=config.id,
                kind=config.kind,
                adapter="fake" if fake else config.adapter,
                distribution=distribution,
                device="cpu" if fake else config.device,
                dtype="float32" if fake else config.dtype,
                weights=config.weights,
                revision=config.revision,
                weights_digest=config.weights_digest,
                fingerprint=infos[config.id].fingerprint,
            )
        )
    return RuntimeResponse(
        runtime_version=1,
        service_version=service_version,
        python_version=platform.python_version(),
        packages=packages,
        fake=fake,
        models=models,
    )
