package config

import (
    "fmt"
    "os"
    "time"

    "gopkg.in/yaml.v3"
)

type Config struct {
    Server struct {
        Host            string        `yaml:"host"`
        Port            int           `yaml:"port"`
        ShutdownTimeout time.Duration `yaml:"shutdownTimeout"`
    } `yaml:"server"`

    Database struct {
        Host     string `yaml:"host"`
        Port     int    `yaml:"port"`
        User     string `yaml:"user"`
        Password string `yaml:"password"`
        DBName   string `yaml:"dbname"`

        MaxConns        int           `yaml:"maxConns"`
        ConnTimeout     time.Duration `yaml:"connTimeout"`
    } `yaml:"database"`
}

func Load(path string) (*Config, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("failed to read config: %w", err)
    }

    var cfg Config
    if err := yaml.Unmarshal(data, &cfg); err != nil {
        return nil, fmt.Errorf("failed to parse config: %w", err)
    }

    return &cfg, nil
}