package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type AdRule struct {
	// MinPeople and MaxPeople define the people count range (-1 means no bound)
	MinPeople int    `yaml:"min_people"`
	MaxPeople int    `yaml:"max_people"`
	AdKey     string `yaml:"ad_key"`
}

type Ad struct {
	// URL can be a local file path served by the app or an external URL
	URL   string `yaml:"url"`
	Label string `yaml:"label"`
}

type Config struct {
	Server struct {
		Port int    `yaml:"port"`
		Host string `yaml:"host"`
	} `yaml:"server"`

	Sensor struct {
		Endpoint       string        `yaml:"endpoint"`
		PollInterval   time.Duration `yaml:"poll_interval"`
		TimeoutSeconds int           `yaml:"timeout_seconds"`
	} `yaml:"sensor"`

	Chrome struct {
		ExecutablePath string `yaml:"executable_path"`
		KioskMode      bool   `yaml:"kiosk_mode"`
		// Extra flags to pass to Chrome
		Flags []string `yaml:"flags"`
	} `yaml:"chrome"`

	// Map of ad key -> Ad
	Ads map[string]Ad `yaml:"ads"`

	// Rules evaluated in order; first match wins
	Rules []AdRule `yaml:"rules"`

	// DefaultAdKey is shown when no rule matches or no people are detected
	DefaultAdKey string `yaml:"default_ad_key"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	cfg.setDefaults()

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

func (c *Config) setDefaults() {
	if c.Server.Port == 0 {
		c.Server.Port = 8080
	}
	if c.Server.Host == "" {
		c.Server.Host = "127.0.0.1"
	}
	if c.Sensor.PollInterval == 0 {
		c.Sensor.PollInterval = 2 * time.Second
	}
	if c.Sensor.TimeoutSeconds == 0 {
		c.Sensor.TimeoutSeconds = 5
	}
}

func (c *Config) validate() error {
	if c.Sensor.Endpoint == "" {
		return fmt.Errorf("sensor.endpoint is required")
	}
	if len(c.Ads) == 0 {
		return fmt.Errorf("at least one ad must be defined")
	}
	if c.DefaultAdKey != "" {
		if _, ok := c.Ads[c.DefaultAdKey]; !ok {
			return fmt.Errorf("default_ad_key %q not found in ads", c.DefaultAdKey)
		}
	}
	for i, rule := range c.Rules {
		if _, ok := c.Ads[rule.AdKey]; !ok {
			return fmt.Errorf("rule[%d]: ad_key %q not found in ads", i, rule.AdKey)
		}
	}
	return nil
}

func (c *Config) ServerAddr() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}

func (c *Config) ServerURL() string {
	return fmt.Sprintf("http://%s:%d", c.Server.Host, c.Server.Port)
}
