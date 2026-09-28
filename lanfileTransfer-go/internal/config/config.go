package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	DefaultSavePath        string `json:"defaultSavePath"`
	AutoReceive            bool   `json:"autoReceive"`
	MaxConcurrentTransfers int    `json:"maxConcurrentTransfers"`
	ChunkSize              int    `json:"chunkSize"`
	ListenPort             int    `json:"listenPort"`
	ServiceName            string `json:"serviceName"`
	ServiceType            string `json:"serviceType"`
	DiscoveryInterval      int    `json:"discoveryInterval"`
}

func DefaultConfig() *Config {
	return &Config{
		DefaultSavePath:        getDefaultSavePath(),
		AutoReceive:            true,
		MaxConcurrentTransfers: 3,
		ChunkSize:              64 * 1024,
		ListenPort:             9876,
		ServiceName:            "LanFileTransfer",
		ServiceType:            "_lanfiletransfer._tcp",
		DiscoveryInterval:      5,
	}
}

func getDefaultSavePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, "Downloads", "LanFileTransfer")
}

func Load(configPath string) *Config {
	cfg := DefaultConfig()

	if configPath == "" {
		configDir, err := os.UserConfigDir()
		if err == nil {
			configPath = filepath.Join(configDir, "LanFileTransfer", "config.json")
		}
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return cfg
	}

	var loaded Config
	if err := json.Unmarshal(data, &loaded); err != nil {
		return cfg
	}

	if loaded.DefaultSavePath != "" {
		cfg.DefaultSavePath = loaded.DefaultSavePath
	}
	cfg.AutoReceive = loaded.AutoReceive
	if loaded.MaxConcurrentTransfers > 0 {
		cfg.MaxConcurrentTransfers = loaded.MaxConcurrentTransfers
	}
	if loaded.ChunkSize > 0 {
		cfg.ChunkSize = loaded.ChunkSize
	}
	if loaded.ListenPort > 0 {
		cfg.ListenPort = loaded.ListenPort
	}
	if loaded.ServiceName != "" {
		cfg.ServiceName = loaded.ServiceName
	}
	if loaded.ServiceType != "" {
		cfg.ServiceType = loaded.ServiceType
	}
	if loaded.DiscoveryInterval > 0 {
		cfg.DiscoveryInterval = loaded.DiscoveryInterval
	}

	return cfg
}

func Save(cfg *Config, configPath string) error {
	if configPath == "" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			return err
		}
		configPath = filepath.Join(configDir, "LanFileTransfer", "config.json")
	}

	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}
