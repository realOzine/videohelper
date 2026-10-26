package config

import (
	"encoding/json"
	"maps"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExampleLoadsEnvironmentAndChannels(t *testing.T) {
	t.Chdir("../..")
	t.Setenv("VH_DB_URL", "postgres://example/database")
	t.Setenv("VH_REDIS_ADDR", "redis:6379")
	t.Setenv("VH_ML_TOKEN", "ml-secret")
	t.Setenv("VH_LLM_API_KEY", "")
	c, err := Load("config/config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if c.Database.URL != "postgres://example/database" || c.Redis.Addr != "redis:6379" || c.ML.Token != "ml-secret" {
		t.Fatal("environment values were not resolved")
	}
	if c.ML.Transcriber != "paraformer-zh@r1" || c.ML.QueryTimeout != 5*time.Second || c.Limits.SoftDeletePurgeAfter != 720*time.Hour || c.Import.Timeout != 2*time.Hour {
		t.Fatal("duration fields were not parsed")
	}
	if len(c.Channels) != 3 || c.Channels["visual"].Active[0] != "wemm-2b@r1" || c.Channels["speech"].Doc != "text" || c.Channels["audio"].Active[0] != "clap-htsat-unfused@r1" || len(c.Import.Platforms["youtube"].Hosts) != 4 {
		t.Fatal("nested channel/platform configuration was lost")
	}
	if err := c.Validate(RoleWorker); err != nil {
		t.Fatalf("worker must not require auth or LLM credentials: %v", err)
	}
	if err := c.Validate(RoleAPI); err == nil {
		t.Fatal("API must require auth credentials")
	}
	c.Auth = AuthConfig{Issuer: "https://auth.example.com", Audience: "videohelper", JWKSURL: "https://auth.example.com/jwks"}
	c.LLM = LLMConfig{}
	c.Database.ADKSchema = ""
	if err := c.Validate(RoleAPI); err != nil {
		t.Fatalf("core search API must run without LLM or ADK configuration: %v", err)
	}
	c.ML.Transcriber = ""
	if err := c.Validate(RoleWorker); err == nil || !strings.Contains(err.Error(), "ml.transcriber") {
		t.Fatalf("missing explicit ASR selection accepted: %v", err)
	}
	c.ML.Transcriber = "paraformer-zh@r1"
	c.Worker.WorkspaceGPULimit = c.Worker.GPUConcurrency
	if err := c.Validate(RoleWorker); err == nil {
		t.Fatal("one workspace must not consume all GPU capacity")
	}
	c.Worker.WorkspaceGPULimit = 1
	c.Limits.Plans["free"] = PlanConfig{IndexedMSMonth: -1}
	if err := c.Validate(RoleWorker); err == nil {
		t.Fatal("negative quotas must be rejected")
	}
	encoded, err := json.Marshal(c)
	if err != nil || strings.Contains(string(encoded), "ml-secret") || strings.Contains(string(encoded), "postgres://") {
		t.Fatal("JSON configuration contains a resolved secret")
	}
}

func TestMigrateOnlyRequiresDatabase(t *testing.T) {
	c := Config{Database: DatabaseConfig{URL: "postgres://db/videohelper", MaxConns: 2}}
	if err := c.Validate(RoleMigrate); err != nil {
		t.Fatal(err)
	}
	c.Database.URL = ""
	if err := c.Validate(RoleMigrate); err == nil {
		t.Fatal("migration must require the database URL")
	}
	if err := c.Validate("typo"); err == nil {
		t.Fatal("unknown role must be rejected")
	}
}

func TestLoadRejectsMalformedConfiguration(t *testing.T) {
	for _, input := range []string{
		"server:\n  address: ':8080'\n",
		"ml:\n  query_timeout: yesterday\n",
		"server: {}\n---\nserver: {}\n",
		"database:\n  url: postgres://secret\n",
	} {
		t.Run(input, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(input), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(path); err == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
}

func TestSearchDefaultsInShippedConfigurations(t *testing.T) {
	t.Chdir("../..")
	for _, path := range []string{"config/local.yaml", "config/config.example.yaml"} {
		t.Run(path, func(t *testing.T) {
			c, err := Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if c.Search.K != 5 || c.Search.RRFK != 60 || c.Search.MergeGapCells != 1 || !maps.Equal(c.Search.ChannelWeights, map[string]float64{"visual": 1, "speech": 1, "audio": 0}) {
				t.Fatalf("unexpected search defaults: %+v", c.Search)
			}
			for name, channel := range c.Channels {
				if channel.Weight != 1 || len(channel.Active) == 0 {
					t.Fatalf("query defaults changed channel indexing configuration %s: %+v", name, channel)
				}
			}
		})
	}
}

func TestValidateSearchChannelDefaults(t *testing.T) {
	for _, test := range []struct {
		name    string
		weights map[string]float64
		base    float64
		valid   bool
	}{
		{name: "omitted", valid: true},
		{name: "partial inherits one", weights: map[string]float64{"visual": 0}, valid: true},
		{name: "audio disabled", weights: map[string]float64{"visual": 1, "speech": 1, "audio": 0}, valid: true},
		{name: "unknown", weights: map[string]float64{"unknown": 1}},
		{name: "negative", weights: map[string]float64{"audio": -1}},
		{name: "NaN", weights: map[string]float64{"audio": math.NaN()}},
		{name: "infinite", weights: map[string]float64{"audio": math.Inf(1)}},
		{name: "overflow", weights: map[string]float64{"audio": math.MaxFloat64}, base: 2},
		{name: "all disabled", weights: map[string]float64{"visual": 0, "speech": 0, "audio": 0}},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := Config{
				Database: DatabaseConfig{URL: "postgres://db/videohelper", MaxConns: 2},
				ML:       MLConfig{Transcriber: "paraformer-zh@r1", Endpoints: []string{"http://ml:8000"}, Token: "test", QueryTimeout: time.Second, EmbedTimeout: time.Second},
				S3:       S3Config{Bucket: "test", Region: "test", PresignTTL: time.Minute},
				Search:   SearchConfig{K: 5, RRFK: 60, MaxLimit: 50, ChannelWeights: test.weights},
				Channels: ChannelsConfig{
					"visual": {Weight: 1, Active: []string{"wemm-2b@r1"}},
					"speech": {Weight: 1, Active: []string{"wemm-2b@r1"}},
					"audio":  {Weight: 1, Active: []string{"clap-htsat-unfused@r1"}},
				},
			}
			if test.base != 0 {
				channel := c.Channels["audio"]
				channel.Weight = test.base
				c.Channels["audio"] = channel
			}
			err := c.Validate(RoleEval)
			if (err == nil) != test.valid {
				t.Fatalf("valid=%v error=%v", test.valid, err)
			}
			if err != nil && !strings.Contains(err.Error(), "search.channel_weights") {
				t.Fatalf("unexpected validation failure: %v", err)
			}
		})
	}
}
