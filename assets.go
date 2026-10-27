// Package videohelper embeds the locked desktop model service sources.
package videohelper

import "embed"

//go:embed ml/pyproject.toml ml/app/*.py ml/app/adapters/*.py ml/app/api/*.py ml/app/core/*.py ml/app/media/*.py ml/app/resources/*.yaml
//go:embed ml/plugins/wemm/pyproject.toml ml/plugins/wemm/videohelper_ml_wemm/*.py
//go:embed ml/plugins/funasr/pyproject.toml ml/plugins/funasr/videohelper_ml_funasr/*.py
//go:embed ml/plugins/clap/pyproject.toml ml/plugins/clap/videohelper_ml_clap/*.py
//go:embed ml/profiles/wemm-mps/pyproject.toml ml/profiles/wemm-mps/uv.lock ml/profiles/wemm-mps/models.yaml
//go:embed ml/profiles/wemm-cpu/pyproject.toml ml/profiles/wemm-cpu/uv.lock ml/profiles/wemm-cpu/models.yaml
//go:embed ml/profiles/wemm-cuda/pyproject.toml ml/profiles/wemm-cuda/uv.lock ml/profiles/wemm-cuda/models.yaml
//go:embed ml/profiles/speech-audio-cpu/pyproject.toml ml/profiles/speech-audio-cpu/uv.lock ml/profiles/speech-audio-cpu/models.yaml
var RuntimeAssets embed.FS
