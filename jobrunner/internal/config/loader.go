package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type ConfigLoader interface {
	Validate() error
}

func LoadConfig[T any, PT interface {
	*T
	ConfigLoader
}](path string) (PT, error) {
	var zero T
	cfg := PT(&zero)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}
