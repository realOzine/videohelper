package config

import (
	"encoding/json"
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
	if c.ML.QueryTimeout != 5*time.Second || c.Limits.SoftDeletePurgeAfter != 720*time.Hour || c.Import.Timeout != 2*time.Hour {
		t.Fatal("duration fields were not parsed")
	}
	if len(c.Channels) != 3 || c.Channels["visual"].Active[0] != "wemm-2b@r1" || len(c.Import.Platforms["youtube"].Hosts) != 4 {
		t.Fatal("nested channel/platform configuration was lost")
	}
	if err := c.Validate(RoleWorker); err != nil {
		t.Fatalf("worker must not require auth or LLM credentials: %v", err)
	}
	if err := c.Validate(RoleAPI); err == nil {
		t.Fatal("API must require auth and LLM credentials")
	}
	c.Auth = AuthConfig{Issuer: "https://auth.example.com", Audience: "videohelper", JWKSURL: "https://auth.example.com/jwks"}
	c.LLM.APIKey = "llm-secret"
	c.LLM.VisionModel = ""
	if err := c.Validate(RoleAPI); err != nil {
		t.Fatalf("disabled inspection must not require a vision model: %v", err)
	}
	c.LLM.InspectClipEnabled = true
	if err := c.Validate(RoleAPI); err == nil {
		t.Fatal("enabled inspection must require a vision model")
	}
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
