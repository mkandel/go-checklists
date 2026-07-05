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
	defaultWebPort = 80
)

// Config holds everything cmd/checklists-server needs to start.
type Config struct {
	Host        string `json:"host"`
	APIPort     int    `json:"api_port"`
	WebPort     int    `json:"web_port"`
	DatabaseURL string `json:"database_url"`
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
