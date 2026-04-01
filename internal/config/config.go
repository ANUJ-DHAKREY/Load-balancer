package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server      ServerConfig      `yaml:"server"`
	Admin       AdminConfig       `yaml:"admin"`
	HealthCheck HealthCheckConfig `yaml:"health_check"`
	Strategy    string            `yaml:"strategy"`
	RateLimit   RateLimitConfig   `yaml:"rate_limit"`
	Backends    []BackendConfig   `yaml:"backends"`
}

type ServerConfig struct {
	Host         string        `yaml:"host"`
	Port         int           `yaml:"port"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
	IdleTimeout  time.Duration `yaml:"idle_timeout"`
	MaxRetries   int           `yaml:"max_retries"`
}

type AdminConfig struct {
	Enabled bool   `yaml:"enabled"`
	Host    string `yaml:"host"`
	Port    int    `yaml:"port"`
}

type HealthCheckConfig struct {
	Interval time.Duration `yaml:"interval"`
	Timeout  time.Duration `yaml:"timeout"`
	Path     string        `yaml:"path"`
}

type RateLimitConfig struct {
	Enabled    bool    `yaml:"enabled"`
	Rate       float64 `yaml:"rate"`
	BurstSize  int     `yaml:"burst_size"`
	PerIP      bool    `yaml:"per_ip"`
	CleanupSec int     `yaml:"cleanup_interval_sec"`
}

type BackendConfig struct {
	URL    string `yaml:"url"`
	Weight int    `yaml:"weight"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	setDefaults(cfg)

	if err := validate(cfg); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return cfg, nil
}

func setDefaults(cfg *Config) {
	if cfg.Server.Host == "" {
		cfg.Server.Host = "0.0.0.0"
	}
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	if cfg.Server.ReadTimeout == 0 {
		cfg.Server.ReadTimeout = 15 * time.Second
	}
	if cfg.Server.WriteTimeout == 0 {
		cfg.Server.WriteTimeout = 15 * time.Second
	}
	if cfg.Server.IdleTimeout == 0 {
		cfg.Server.IdleTimeout = 60 * time.Second
	}
	if cfg.Server.MaxRetries == 0 {
		cfg.Server.MaxRetries = 3
	}
	if cfg.Admin.Host == "" {
		cfg.Admin.Host = "127.0.0.1"
	}
	if cfg.Admin.Port == 0 {
		cfg.Admin.Port = 9090
	}
	if cfg.HealthCheck.Interval == 0 {
		cfg.HealthCheck.Interval = 10 * time.Second
	}
	if cfg.HealthCheck.Timeout == 0 {
		cfg.HealthCheck.Timeout = 5 * time.Second
	}
	if cfg.HealthCheck.Path == "" {
		cfg.HealthCheck.Path = "/health"
	}
	if cfg.Strategy == "" {
		cfg.Strategy = "round-robin"
	}
	if cfg.RateLimit.Rate == 0 {
		cfg.RateLimit.Rate = 100
	}
	if cfg.RateLimit.BurstSize == 0 {
		cfg.RateLimit.BurstSize = 200
	}
	if cfg.RateLimit.CleanupSec == 0 {
		cfg.RateLimit.CleanupSec = 300
	}

	for i := range cfg.Backends {
		if cfg.Backends[i].Weight == 0 {
			cfg.Backends[i].Weight = 1
		}
	}
}

func validate(cfg *Config) error {
	validStrategies := map[string]bool{
		"round-robin":          true,
		"weighted-round-robin": true,
		"least-connections":    true,
	}
	if !validStrategies[cfg.Strategy] {
		return fmt.Errorf("unknown strategy %q; valid: round-robin, weighted-round-robin, least-connections", cfg.Strategy)
	}
	if len(cfg.Backends) == 0 {
		return fmt.Errorf("at least one backend is required")
	}
	if cfg.Server.Port < 1 || cfg.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", cfg.Server.Port)
	}
	for i, b := range cfg.Backends {
		if b.URL == "" {
			return fmt.Errorf("backend[%d]: url is required", i)
		}
		if b.Weight < 1 {
			return fmt.Errorf("backend[%d]: weight must be >= 1", i)
		}
	}
	return nil
}
