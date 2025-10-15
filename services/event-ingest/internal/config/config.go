package config

import (
	"os"
	"strconv"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Server   ServerConfig   `toml:"server"`
	Events   EventsConfig   `toml:"events"`
	Logging  LoggingConfig  `toml:"logging"`
	Pipeline PipelineConfig `toml:"pipeline"`
}

type ServerConfig struct {
	Host         string `toml:"host"`
	Port         int    `toml:"port"`
	ReadTimeout  int    `toml:"read_timeout"`
	WriteTimeout int    `toml:"write_timeout"`
	IdleTimeout  int    `toml:"idle_timeout"`
}

type EventsConfig struct {
	MaxPayloadSize   int `toml:"max_payload_size"`
	ValidationTimeout int `toml:"validation_timeout"`
	MaxBatchSize     int `toml:"max_batch_size"`
	GenerationInterval int `toml:"generation_interval"`
	EventsPerBatch   int `toml:"events_per_batch"`
}

type LoggingConfig struct {
	Level  string `toml:"level"`
	Format string `toml:"format"`
}

type PipelineConfig struct {
	URL         string `toml:"url"`
	ShardsCount int    `toml:"shards_count"`
}

func Load() (*Config, error) {
	// Создаем конфигурацию с значениями по умолчанию
	config := &Config{
		Server: ServerConfig{
			Host:         "0.0.0.0",
			Port:         8080,
			ReadTimeout:  30,
			WriteTimeout: 30,
			IdleTimeout:  120,
		},
		Events: EventsConfig{
			MaxPayloadSize:     1048576,
			ValidationTimeout:  5,
			MaxBatchSize:       1000,
			GenerationInterval: 10,
			EventsPerBatch:     5,
		},
		Logging: LoggingConfig{
			Level:  "INFO",
			Format: "json",
		},
		Pipeline: PipelineConfig{
			URL:         "http://pipeline-api:8082/api/v1/pipeline",
			ShardsCount: 2,
		},
	}

	// Пытаемся загрузить конфигурационный файл
	configPaths := []string{
		"./config/app.toml",
		"/app/config/app.toml",
	}

	for _, path := range configPaths {
		if _, err := toml.DecodeFile(path, config); err == nil {
			break
		}
	}

	// Переопределяем значения из переменных окружения
	overrideFromEnv(config)

	return config, nil
}


func overrideFromEnv(config *Config) {
	if val := os.Getenv("SERVER_PORT"); val != "" {
		if port, err := strconv.Atoi(val); err == nil {
			config.Server.Port = port
		}
	}
	if val := os.Getenv("PIPELINE_URL"); val != "" {
		config.Pipeline.URL = val
	}
	if val := os.Getenv("PIPELINE_SHARDS_COUNT"); val != "" {
		if n, err := strconv.Atoi(val); err == nil {
			config.Pipeline.ShardsCount = n
		}
	}
}
