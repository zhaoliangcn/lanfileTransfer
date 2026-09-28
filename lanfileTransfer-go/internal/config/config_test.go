package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.DefaultSavePath == "" {
		t.Errorf("DefaultSavePath should not be empty")
	}
	if cfg.AutoReceive != true {
		t.Errorf("AutoReceive should default to true")
	}
	if cfg.MaxConcurrentTransfers != 3 {
		t.Errorf("MaxConcurrentTransfers should default to 3, got %d", cfg.MaxConcurrentTransfers)
	}
	if cfg.ChunkSize != 64*1024 {
		t.Errorf("ChunkSize should default to 65536, got %d", cfg.ChunkSize)
	}
	if cfg.ListenPort != 9876 {
		t.Errorf("ListenPort should default to 9876, got %d", cfg.ListenPort)
	}
	if cfg.ServiceName != "LanFileTransfer" {
		t.Errorf("ServiceName should default to 'LanFileTransfer', got '%s'", cfg.ServiceName)
	}
	if cfg.DiscoveryInterval != 5 {
		t.Errorf("DiscoveryInterval should default to 5, got %d", cfg.DiscoveryInterval)
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")

	original := &Config{
		DefaultSavePath:        "/tmp/saves",
		AutoReceive:            false,
		MaxConcurrentTransfers: 5,
		ChunkSize:              128 * 1024,
		ListenPort:             9877,
		ServiceName:            "MyApp",
		ServiceType:            "_myapp._tcp",
		DiscoveryInterval:      10,
	}

	if err := Save(original, configPath); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Errorf("config file should exist after save")
	}

	loaded := Load(configPath)

	if loaded.DefaultSavePath != original.DefaultSavePath {
		t.Errorf("DefaultSavePath mismatch: got '%s', want '%s'", loaded.DefaultSavePath, original.DefaultSavePath)
	}
	if loaded.AutoReceive != original.AutoReceive {
		t.Errorf("AutoReceive mismatch: got %v, want %v", loaded.AutoReceive, original.AutoReceive)
	}
	if loaded.MaxConcurrentTransfers != original.MaxConcurrentTransfers {
		t.Errorf("MaxConcurrentTransfers mismatch: got %d, want %d", loaded.MaxConcurrentTransfers, original.MaxConcurrentTransfers)
	}
	if loaded.ChunkSize != original.ChunkSize {
		t.Errorf("ChunkSize mismatch: got %d, want %d", loaded.ChunkSize, original.ChunkSize)
	}
	if loaded.ListenPort != original.ListenPort {
		t.Errorf("ListenPort mismatch: got %d, want %d", loaded.ListenPort, original.ListenPort)
	}
	if loaded.ServiceName != original.ServiceName {
		t.Errorf("ServiceName mismatch: got '%s', want '%s'", loaded.ServiceName, original.ServiceName)
	}
	if loaded.DiscoveryInterval != original.DiscoveryInterval {
		t.Errorf("DiscoveryInterval mismatch: got %d, want %d", loaded.DiscoveryInterval, original.DiscoveryInterval)
	}
}

func TestLoadNonExistentConfig(t *testing.T) {
	cfg := Load("/nonexistent/path/config.json")

	if cfg == nil {
		t.Fatal("Load should return default config for non-existent path")
	}
	if cfg.MaxConcurrentTransfers != 3 {
		t.Errorf("should return default config, got MaxConcurrentTransfers=%d", cfg.MaxConcurrentTransfers)
	}
}

func TestLoadEmptyConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "empty_config.json")

	if err := os.WriteFile(configPath, []byte("{}"), 0644); err != nil {
		t.Fatalf("failed to write empty config: %v", err)
	}

	cfg := Load(configPath)

	if cfg.MaxConcurrentTransfers != 3 {
		t.Errorf("should use defaults for empty config, got MaxConcurrentTransfers=%d", cfg.MaxConcurrentTransfers)
	}
	if cfg.DefaultSavePath == "" {
		t.Errorf("DefaultSavePath should be set from defaults")
	}
}

func TestLoadPartialConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "partial_config.json")

	partialJSON := `{"maxConcurrentTransfers": 8}`
	if err := os.WriteFile(configPath, []byte(partialJSON), 0644); err != nil {
		t.Fatalf("failed to write partial config: %v", err)
	}

	cfg := Load(configPath)

	if cfg.MaxConcurrentTransfers != 8 {
		t.Errorf("MaxConcurrentTransfers should be 8, got %d", cfg.MaxConcurrentTransfers)
	}
	if cfg.ChunkSize != 64*1024 {
		t.Errorf("ChunkSize should use default 65536, got %d", cfg.ChunkSize)
	}
}

func TestSaveCreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	nestedPath := filepath.Join(dir, "nested", "deep", "dir", "config.json")

	cfg := DefaultConfig()
	if err := Save(cfg, nestedPath); err != nil {
		t.Fatalf("Save to nested path failed: %v", err)
	}

	if _, err := os.Stat(nestedPath); os.IsNotExist(err) {
		t.Errorf("config file should exist after save to nested path")
	}
}

func TestConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "roundtrip.json")

	original := DefaultConfig()
	original.ListenPort = 9999
	original.AutoReceive = false
	original.MaxConcurrentTransfers = 10

	if err := Save(original, configPath); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded := Load(configPath)

	if loaded.ListenPort != 9999 {
		t.Errorf("ListenPort round-trip failed: got %d", loaded.ListenPort)
	}
	if loaded.AutoReceive != false {
		t.Errorf("AutoReceive round-trip failed: got %v", loaded.AutoReceive)
	}
	if loaded.MaxConcurrentTransfers != 10 {
		t.Errorf("MaxConcurrentTransfers round-trip failed: got %d", loaded.MaxConcurrentTransfers)
	}
}
