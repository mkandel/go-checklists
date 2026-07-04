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

const defaultPort = 8080

// Config holds everything cmd/checklists-server needs to start.
type Config struct {
	Host        string `json:"host"`
	Port        int    `json:"port"`
	DatabaseURL string `json:"database_url"`
}

// Addr returns the host:port string for http.ListenAndServe.
func (c *Config) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// Load builds a Config from, in increasing order of precedence: a JSON
// config file, environment variables, and command-line flags. args is
// typically os.Args[1:]; lookupEnv is typically os.LookupEnv.
func Load(args []string, lookupEnv func(string) (string, bool)) (*Config, error) {
	fs := flag.NewFlagSet("checklists-server", flag.ContinueOnError)
	configPath := fs.String("config", "", "path to a JSON config file")
	fs.StringVar(configPath, "c", "", "shorthand for -config")
	host := fs.String("host", "", "listen host")
	port := fs.Int("port", 0, "listen port")
	databaseURL := fs.String("database-url", "", "Postgres connection string")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	cfg := &Config{Port: defaultPort}

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
	if v, ok := lookupEnv("LISTEN_PORT"); ok {
		p, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("config: LISTEN_PORT: %w", err)
		}
		cfg.Port = p
	}
	if v, ok := lookupEnv("DATABASE_URL"); ok {
		cfg.DatabaseURL = v
	}

	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "host":
			cfg.Host = *host
		case "port":
			cfg.Port = *port
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
