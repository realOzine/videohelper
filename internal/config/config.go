package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server       ServerConfig   `yaml:"server"`
	Database     DatabaseConfig `yaml:"database"`
	Redis        RedisConfig    `yaml:"redis"`
	S3           S3Config       `yaml:"s3"`
	Auth         AuthConfig     `yaml:"auth"`
	ML           MLConfig       `yaml:"ml"`
	LLM          LLMConfig      `yaml:"llm"`
	Worker       WorkerConfig   `yaml:"worker"`
	Media        MediaConfig    `yaml:"media"`
	Search       SearchConfig   `yaml:"search"`
	Import       ImportConfig   `yaml:"import"`
	Limits       LimitsConfig   `yaml:"limits"`
	ChannelsFile string         `yaml:"channels_file"`
	Channels     ChannelsConfig `yaml:"-"`
}

type ServerConfig struct {
	Addr        string   `yaml:"addr"`
	CORSOrigins []string `yaml:"cors_origins"`
}

type DatabaseConfig struct {
	URLEnv    string `yaml:"url_env"`
	URL       string `yaml:"-" json:"-"`
	MaxConns  int    `yaml:"max_conns"`
	ADKSchema string `yaml:"adk_schema"`
}

type RedisConfig struct {
	AddrEnv string `yaml:"addr_env"`
	Addr    string `yaml:"-" json:"-"`
}

// S3 credentials use the AWS SDK's environment/role credential chain.
type S3Config struct {
	Bucket     string        `yaml:"bucket"`
	Region     string        `yaml:"region"`
	Endpoint   string        `yaml:"endpoint"`
	PresignTTL time.Duration `yaml:"presign_ttl"`
}

type AuthConfig struct {
	Issuer   string `yaml:"issuer"`
	Audience string `yaml:"audience"`
	JWKSURL  string `yaml:"jwks_url"`
}

type MLConfig struct {
	Endpoints           []string      `yaml:"endpoints"`
	TokenEnv            string        `yaml:"token_env"`
	Token               string        `yaml:"-" json:"-"`
	QueryTimeout        time.Duration `yaml:"query_timeout"`
	EmbedTimeout        time.Duration `yaml:"embed_timeout"`
	EmbedBatch          int           `yaml:"embed_batch"`
	TranscribeChunkMS   int           `yaml:"transcribe_chunk_ms"`
	TranscribeOverlapMS int           `yaml:"transcribe_overlap_ms"`
	TranscribeTimeout   time.Duration `yaml:"transcribe_timeout"`
}

type LLMConfig struct {
	BaseURL             string        `yaml:"base_url"`
	APIKeyEnv           string        `yaml:"api_key_env"`
	APIKey              string        `yaml:"-" json:"-"`
	ChatModel           string        `yaml:"chat_model"`
	VisionModel         string        `yaml:"vision_model"`
	Thinking            bool          `yaml:"thinking"`
	MaxToolCallsPerTurn int           `yaml:"max_tool_calls_per_turn"`
	TurnTimeout         time.Duration `yaml:"turn_timeout"`
	InspectClipEnabled  bool          `yaml:"inspect_clip_enabled"`
}

type WorkerConfig struct {
	CPUConcurrency    int           `yaml:"cpu_concurrency"`
	GPUConcurrency    int           `yaml:"gpu_concurrency"`
	WorkspaceGPULimit int           `yaml:"workspace_gpu_limit"`
	MaxRetry          int           `yaml:"max_retry"`
	ReconcileEvery    time.Duration `yaml:"reconcile_every"`
}

type MediaConfig struct {
	NormalizedVideoMaxHeight  int    `yaml:"normalized_video_max_height"`
	NormalizedVideoGOPSeconds int    `yaml:"normalized_video_gop_seconds"`
	ThumbWidth                int    `yaml:"thumb_width"`
	SpriteCols                int    `yaml:"sprite_cols"`
	SpriteRows                int    `yaml:"sprite_rows"`
	TmpDir                    string `yaml:"tmp_dir"`
}

type SearchConfig struct {
	K              int                `yaml:"k"`
	RRFK           float64            `yaml:"rrf_k"`
	MergeGapCells  int                `yaml:"merge_gap_cells"`
	MaxLimit       int                `yaml:"max_limit"`
	ChannelWeights map[string]float64 `yaml:"channel_weights"`
}

type ImportConfig struct {
	YTDLPPath     string                    `yaml:"ytdlp_path"`
	AllowDirect   bool                      `yaml:"allow_direct"`
	MaxHeight     int                       `yaml:"max_height"`
	MaxDurationMS int                       `yaml:"max_duration_ms"`
	MaxBytes      int64                     `yaml:"max_bytes"`
	Timeout       time.Duration             `yaml:"timeout"`
	Platforms     map[string]PlatformConfig `yaml:"platforms"`
}

type PlatformConfig struct {
	Hosts          []string `yaml:"hosts"`
	Extractors     []string `yaml:"extractors"`
	MaxConcurrency int      `yaml:"max_concurrency"`
}

type LimitsConfig struct {
	UploadPartBytes      int64                 `yaml:"upload_part_bytes"`
	SoftDeletePurgeAfter time.Duration         `yaml:"soft_delete_purge_after"`
	Plans                map[string]PlanConfig `yaml:"plans"`
}

type PlanConfig struct {
	IndexedMSMonth int64 `yaml:"indexed_ms_month"`
	LLMTokensDay   int64 `yaml:"llm_tokens_day"`
}

type ChannelsConfig map[string]ChannelConfig

type ChannelConfig struct {
	Doc    string   `yaml:"doc"`
	Seg    string   `yaml:"seg"`
	Weight float64  `yaml:"weight"`
	Active []string `yaml:"active"`
	Shadow []string `yaml:"shadow"`
}

// Load reads paths relative to the process working directory, as in the example.
// Required runtime inputs are checked separately by Validate for the selected role.
func Load(path string) (Config, error) {
	var c Config
	if err := decode(path, &c); err != nil {
		return Config{}, err
	}
	c.Database.URL = os.Getenv(c.Database.URLEnv)
	c.Redis.Addr = os.Getenv(c.Redis.AddrEnv)
	c.ML.Token = os.Getenv(c.ML.TokenEnv)
	c.LLM.APIKey = os.Getenv(c.LLM.APIKeyEnv)
	if c.ChannelsFile != "" {
		var channels struct {
			Channels ChannelsConfig `yaml:"channels"`
		}
		if err := decode(c.ChannelsFile, &channels); err != nil {
			return Config{}, err
		}
		c.Channels = channels.Channels
	}
	return c, nil
}

func decode(path string, dst any) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open configuration %s: %w", path, err)
	}
	defer f.Close()
	d := yaml.NewDecoder(f)
	d.KnownFields(true)
	if err := d.Decode(dst); err != nil {
		return fmt.Errorf("decode configuration %s: %w", path, err)
	}
	var extra any
	if err := d.Decode(&extra); !errors.Is(err, io.EOF) {
		return fmt.Errorf("configuration %s must contain one YAML document", path)
	}
	return nil
}
