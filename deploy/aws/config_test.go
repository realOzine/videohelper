package aws_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ozine/videohelper/internal/config"
)

func TestDemoConfigValidForEveryCommandRole(t *testing.T) {
	t.Setenv("VH_DB_URL", "postgres://videohelper:test@postgres/videohelper")
	t.Setenv("VH_REDIS_ADDR", "redis:6379")
	t.Setenv("VH_ML_TOKEN", "test-only")
	data, err := os.ReadFile("config.template.yaml")
	if err != nil {
		t.Fatal(err)
	}
	channels, err := filepath.Abs("channels.yaml")
	if err != nil {
		t.Fatal(err)
	}
	content := strings.NewReplacer("__MEDIA_BUCKET__", "demo-media", "__AWS_REGION__", "ap-southeast-1", "/etc/videohelper/channels.yaml", channels).Replace(string(data))
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range []config.Role{config.RoleAPI, config.RoleWorker, config.RoleMigrate, config.RoleSpace, config.RoleEval} {
		if err := cfg.Validate(role); err != nil {
			t.Errorf("%s: %v", role, err)
		}
	}
	for _, channel := range []string{"visual", "speech"} {
		spaces := cfg.Channels[channel].Active
		if len(spaces) != 1 || spaces[0] != "wemm-2b-cpu@r1" {
			t.Fatalf("%s must use the CPU space, got %v", channel, spaces)
		}
		if endpoint := cfg.ML.ModelEndpoints[spaces[0]]; endpoint != "http://wemm:8000" {
			t.Errorf("%s CPU space bound to unexpected endpoint %q", channel, endpoint)
		}
	}
}
