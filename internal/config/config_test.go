package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mkandel/go-checklists/internal/config"
)

func fakeEnv(vars map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		v, ok := vars[key]
		return v, ok
	}
}

func writeConfigFile(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}
	return path
}

func TestLoad_Defaults(t *testing.T) {
	cfg, err := config.Load(nil, fakeEnv(map[string]string{"DATABASE_URL": "postgres://db"}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != 8080 {
		t.Fatalf("Port = %d, want 8080", cfg.Port)
	}
	if cfg.Host != "" {
		t.Fatalf("Host = %q, want empty", cfg.Host)
	}
	if cfg.DatabaseURL != "postgres://db" {
		t.Fatalf("DatabaseURL = %q, want postgres://db", cfg.DatabaseURL)
	}
}

func TestLoad_FileSetsValues(t *testing.T) {
	path := writeConfigFile(t, `{"host":"0.0.0.0","port":9090,"database_url":"postgres://file"}`)

	cfg, err := config.Load([]string{"-config", path}, fakeEnv(nil))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Host != "0.0.0.0" || cfg.Port != 9090 || cfg.DatabaseURL != "postgres://file" {
		t.Fatalf("cfg = %+v, want host=0.0.0.0 port=9090 database_url=postgres://file", cfg)
	}
}

func TestLoad_EnvOverridesFile(t *testing.T) {
	path := writeConfigFile(t, `{"port":9090,"database_url":"postgres://file"}`)

	cfg, err := config.Load([]string{"-config", path}, fakeEnv(map[string]string{"LISTEN_PORT": "9091"}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != 9091 {
		t.Fatalf("Port = %d, want 9091 (env should override file)", cfg.Port)
	}
}

func TestLoad_FlagOverridesEnvAndFile(t *testing.T) {
	path := writeConfigFile(t, `{"port":9090,"database_url":"postgres://file"}`)

	cfg, err := config.Load(
		[]string{"-config", path, "-port", "9092"},
		fakeEnv(map[string]string{"LISTEN_PORT": "9091"}),
	)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != 9092 {
		t.Fatalf("Port = %d, want 9092 (flag should win over env and file)", cfg.Port)
	}
}

func TestLoad_MissingDatabaseURL_Error(t *testing.T) {
	_, err := config.Load(nil, fakeEnv(nil))
	if err == nil {
		t.Fatal("expected an error when DATABASE_URL is unset everywhere")
	}
}

func TestLoad_ConfigFlagBadPath_Error(t *testing.T) {
	_, err := config.Load([]string{"-config", "/does/not/exist.json"}, fakeEnv(nil))
	if err == nil {
		t.Fatal("expected an error for a nonexistent -config path")
	}
}

func TestLoad_ShorthandCFlag(t *testing.T) {
	path := writeConfigFile(t, `{"port":9094,"database_url":"postgres://file"}`)

	cfg, err := config.Load([]string{"-c", path}, fakeEnv(nil))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != 9094 {
		t.Fatalf("Port = %d, want 9094 (-c should be a shorthand for -config)", cfg.Port)
	}
}

func TestLoad_ChecklistsConfigEnvHonored(t *testing.T) {
	path := writeConfigFile(t, `{"port":9093,"database_url":"postgres://file"}`)

	cfg, err := config.Load(nil, fakeEnv(map[string]string{"CHECKLISTS_CONFIG": path}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != 9093 {
		t.Fatalf("Port = %d, want 9093 (CHECKLISTS_CONFIG env should locate the file)", cfg.Port)
	}
}

func TestLoad_BadListenPortEnv_Error(t *testing.T) {
	_, err := config.Load(nil, fakeEnv(map[string]string{
		"DATABASE_URL": "postgres://db",
		"LISTEN_PORT":  "not-a-number",
	}))
	if err == nil {
		t.Fatal("expected an error for a non-numeric LISTEN_PORT")
	}
	if !strings.Contains(err.Error(), "LISTEN_PORT") {
		t.Fatalf("error = %v, want it to mention LISTEN_PORT", err)
	}
}
