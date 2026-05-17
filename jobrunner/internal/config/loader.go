package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type ConfigLoader interface {
    Validate() error
}

func LoadConfig[T interface {
    *T
    ConfigLoader
}](path string) (T, error) {
    var cfg T
    data, err := os.ReadFile(path)
    if err != nil {
        return cfg, fmt.Errorf("failed to read file: %w", err)
    }
    
    if err := yaml.Unmarshal(data, cfg); err != nil {
        return cfg, fmt.Errorf("failed to parse YAML: %w", err)
    }
        
    if err := cfg.Validate(); err != nil {
        return cfg, fmt.Errorf("validation failed: %w", err)
    }
    
    return cfg, nil
}