// Package config contains the private local-model client settings.
package config

import "time"

type MLConfig struct {
	Endpoints         []string
	Token             string `json:"-"`
	QueryTimeout      time.Duration
	EmbedTimeout      time.Duration
	TranscribeTimeout time.Duration
}
