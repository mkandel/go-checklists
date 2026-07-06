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
	if cfg.APIPort != 8080 {
		t.Fatalf("APIPort = %d, want 8080", cfg.APIPort)
	}
	if cfg.WebPort != 8081 {
		t.Fatalf("WebPort = %d, want 8081", cfg.WebPort)
	}
	if cfg.Host != "" {
		t.Fatalf("Host = %q, want empty", cfg.Host)
	}
	if cfg.DatabaseURL != "postgres://db" {
		t.Fatalf("DatabaseURL = %q, want postgres://db", cfg.DatabaseURL)
	}
	if cfg.TrustProxy {
		t.Fatal("TrustProxy = true, want false by default")
	}
}

func TestLoad_FileSetsValues(t *testing.T) {
	path := writeConfigFile(t, `{"host":"0.0.0.0","api_port":9090,"web_port":9095,"database_url":"postgres://file"}`)

	cfg, err := config.Load([]string{"-config", path}, fakeEnv(nil))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Host != "0.0.0.0" || cfg.APIPort != 9090 || cfg.WebPort != 9095 || cfg.DatabaseURL != "postgres://file" {
		t.Fatalf("cfg = %+v, want host=0.0.0.0 api_port=9090 web_port=9095 database_url=postgres://file", cfg)
	}
}

func TestLoad_EnvOverridesFile(t *testing.T) {
	path := writeConfigFile(t, `{"api_port":9090,"web_port":9095,"database_url":"postgres://file"}`)

	cfg, err := config.Load([]string{"-config", path}, fakeEnv(map[string]string{"API_PORT": "9091", "WEB_PORT": "9096"}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.APIPort != 9091 {
		t.Fatalf("APIPort = %d, want 9091 (env should override file)", cfg.APIPort)
	}
	if cfg.WebPort != 9096 {
		t.Fatalf("WebPort = %d, want 9096 (env should override file)", cfg.WebPort)
	}
}

func TestLoad_FlagOverridesEnvAndFile(t *testing.T) {
	path := writeConfigFile(t, `{"api_port":9090,"web_port":9095,"database_url":"postgres://file"}`)

	cfg, err := config.Load(
		[]string{"-config", path, "-api-port", "9092", "-web-port", "9097"},
		fakeEnv(map[string]string{"API_PORT": "9091", "WEB_PORT": "9096"}),
	)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.APIPort != 9092 {
		t.Fatalf("APIPort = %d, want 9092 (flag should win over env and file)", cfg.APIPort)
	}
	if cfg.WebPort != 9097 {
		t.Fatalf("WebPort = %d, want 9097 (flag should win over env and file)", cfg.WebPort)
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
	path := writeConfigFile(t, `{"api_port":9094,"database_url":"postgres://file"}`)

	cfg, err := config.Load([]string{"-c", path}, fakeEnv(nil))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.APIPort != 9094 {
		t.Fatalf("APIPort = %d, want 9094 (-c should be a shorthand for -config)", cfg.APIPort)
	}
}

func TestLoad_ChecklistsConfigEnvHonored(t *testing.T) {
	path := writeConfigFile(t, `{"api_port":9093,"database_url":"postgres://file"}`)

	cfg, err := config.Load(nil, fakeEnv(map[string]string{"CHECKLISTS_CONFIG": path}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.APIPort != 9093 {
		t.Fatalf("APIPort = %d, want 9093 (CHECKLISTS_CONFIG env should locate the file)", cfg.APIPort)
	}
}

func TestLoad_BadAPIPortEnv_Error(t *testing.T) {
	_, err := config.Load(nil, fakeEnv(map[string]string{
		"DATABASE_URL": "postgres://db",
		"API_PORT":     "not-a-number",
	}))
	if err == nil {
		t.Fatal("expected an error for a non-numeric API_PORT")
	}
	if !strings.Contains(err.Error(), "API_PORT") {
		t.Fatalf("error = %v, want it to mention API_PORT", err)
	}
}

func TestLoad_TrustProxy_FileEnvFlagPrecedence(t *testing.T) {
	path := writeConfigFile(t, `{"database_url":"postgres://file","trust_proxy":true}`)

	cfg, err := config.Load([]string{"-config", path}, fakeEnv(nil))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.TrustProxy {
		t.Fatal("TrustProxy = false, want true (from config file)")
	}

	cfg, err = config.Load([]string{"-config", path}, fakeEnv(map[string]string{"TRUST_PROXY": "false"}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.TrustProxy {
		t.Fatal("TrustProxy = true, want false (env should override file)")
	}

	cfg, err = config.Load(
		[]string{"-config", path, "-trust-proxy=true"},
		fakeEnv(map[string]string{"TRUST_PROXY": "false"}),
	)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.TrustProxy {
		t.Fatal("TrustProxy = false, want true (flag should win over env and file)")
	}
}

func TestLoad_BadTrustProxyEnv_Error(t *testing.T) {
	_, err := config.Load(nil, fakeEnv(map[string]string{
		"DATABASE_URL": "postgres://db",
		"TRUST_PROXY":  "not-a-bool",
	}))
	if err == nil {
		t.Fatal("expected an error for a non-boolean TRUST_PROXY")
	}
	if !strings.Contains(err.Error(), "TRUST_PROXY") {
		t.Fatalf("error = %v, want it to mention TRUST_PROXY", err)
	}
}

func TestLoad_BadWebPortEnv_Error(t *testing.T) {
	_, err := config.Load(nil, fakeEnv(map[string]string{
		"DATABASE_URL": "postgres://db",
		"WEB_PORT":     "not-a-number",
	}))
	if err == nil {
		t.Fatal("expected an error for a non-numeric WEB_PORT")
	}
	if !strings.Contains(err.Error(), "WEB_PORT") {
		t.Fatalf("error = %v, want it to mention WEB_PORT", err)
	}
}

func TestString_RedactsDatabasePassword(t *testing.T) {
	cfg, err := config.Load(nil, fakeEnv(map[string]string{
		"DATABASE_URL": "postgres://checklists:supersecret@localhost:5432/checklists?sslmode=disable",
	}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	s := cfg.String()
	if strings.Contains(s, "supersecret") {
		t.Fatalf("String() leaked the password: %s", s)
	}
	if !strings.Contains(s, "checklists:xxxxx@") {
		t.Fatalf("String() = %s, want a redacted checklists:xxxxx@ user info", s)
	}
}

func TestString_NoPasswordLeftUnchanged(t *testing.T) {
	cfg, err := config.Load(nil, fakeEnv(map[string]string{
		"DATABASE_URL": "postgres://localhost:5432/checklists?sslmode=disable",
	}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	s := cfg.String()
	if !strings.Contains(s, "postgres://localhost:5432/checklists?sslmode=disable") {
		t.Fatalf("String() = %s, want the passwordless URL unchanged", s)
	}
}
