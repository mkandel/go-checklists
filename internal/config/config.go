// Package config loads server configuration from a JSON file, environment
// variables, and command-line flags, layered so each source overrides the
// previous one: file, then env, then flags.
package config

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
)

const (
	defaultAPIPort = 8080
	defaultWebPort = 8081
)

// Config holds everything cmd/checklists-server needs to start.
type Config struct {
	Host        string `json:"host"`
	APIPort     int    `json:"api_port"`
	WebPort     int    `json:"web_port"`
	DatabaseURL string `json:"database_url"`

	// TrustProxy, when true, makes the server honor X-Forwarded-Proto (for
	// the session/CSRF cookies' Secure flag) and X-Forwarded-For (for
	// per-client login rate limiting) — safe only when a reverse proxy the
	// server operator controls (e.g. Caddy) sits in front and sets these
	// headers itself; otherwise a client could spoof them.
	TrustProxy bool `json:"trust_proxy"`

	// NotificationsEnabled toggles the notifications feature (badge, SSE
	// stream, list page, and both API/web routes) on or off without removing
	// any of the underlying code. Defaults to false — flip on once the
	// feature's UI/performance work (see internal/notify) is revisited.
	NotificationsEnabled bool `json:"notifications_enabled"`
}

// APIAddr returns the host:port string the JSON API listens on.
func (c *Config) APIAddr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.APIPort)
}

// WebAddr returns the host:port string the browser UI listens on.
func (c *Config) WebAddr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.WebPort)
}

// Load builds a Config from, in increasing order of precedence: a JSON
// config file, environment variables, and command-line flags. args is
// typically os.Args[1:]; lookupEnv is typically os.LookupEnv.
func Load(args []string, lookupEnv func(string) (string, bool)) (*Config, error) {
	fs := flag.NewFlagSet("checklists-server", flag.ContinueOnError)
	configPath := fs.String("config", "", "path to a JSON config file")
	fs.StringVar(configPath, "c", "", "shorthand for -config")
	host := fs.String("host", "", "listen host")
	apiPort := fs.Int("api-port", 0, "JSON API listen port")
	webPort := fs.Int("web-port", 0, "browser UI listen port")
	databaseURL := fs.String("database-url", "", "Postgres connection string")
	trustProxy := fs.Bool("trust-proxy", false, "trust X-Forwarded-Proto/X-Forwarded-For from a reverse proxy in front of this server")
	notificationsEnabled := fs.Bool("notifications-enabled", false, "enable the notifications feature (badge, SSE stream, list page)")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	cfg := &Config{APIPort: defaultAPIPort, WebPort: defaultWebPort}

	path := *configPath
	if path == "" {
		if v, ok := lookupEnv("CHECKLISTS_CONFIG"); ok {
			path = v
		}
	}
	if path != "" {
		if err := loadFile(cfg, path); err != nil {
			return nil, err
		}
	}

	if v, ok := lookupEnv("LISTEN_HOST"); ok {
		cfg.Host = v
	}
	if v, ok := lookupEnv("API_PORT"); ok {
		p, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("config: API_PORT: %w", err)
		}
		cfg.APIPort = p
	}
	if v, ok := lookupEnv("WEB_PORT"); ok {
		p, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("config: WEB_PORT: %w", err)
		}
		cfg.WebPort = p
	}
	if v, ok := lookupEnv("DATABASE_URL"); ok {
		cfg.DatabaseURL = v
	}
	if v, ok := lookupEnv("TRUST_PROXY"); ok {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("config: TRUST_PROXY: %w", err)
		}
		cfg.TrustProxy = b
	}
	if v, ok := lookupEnv("NOTIFICATIONS_ENABLED"); ok {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("config: NOTIFICATIONS_ENABLED: %w", err)
		}
		cfg.NotificationsEnabled = b
	}

	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "host":
			cfg.Host = *host
		case "api-port":
			cfg.APIPort = *apiPort
		case "web-port":
			cfg.WebPort = *webPort
		case "database-url":
			cfg.DatabaseURL = *databaseURL
		case "trust-proxy":
			cfg.TrustProxy = *trustProxy
		case "notifications-enabled":
			cfg.NotificationsEnabled = *notificationsEnabled
		}
	})

	if cfg.DatabaseURL == "" {
		return nil, errors.New("config: DATABASE_URL is required (via config file, env, or -database-url flag)")
	}
	return cfg, nil
}

func loadFile(cfg *Config, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("config: read %s: %w", path, err)
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("config: parse %s: %w", path, err)
	}
	return nil
}
