// Package videohelper embeds the locked desktop model service sources.
package videohelper

import "embed"

//go:embed ml/pyproject.toml ml/app/*.py ml/app/*/*.py ml/app/resources/*.yaml ml/plugins/*/pyproject.toml ml/plugins/*/*/*.py ml/profiles/*/pyproject.toml ml/profiles/*/uv.lock ml/profiles/*/*.yaml
var RuntimeAssets embed.FS
