package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const (
	ServiceTypeBranpg     = "branpg"
	ServiceTypeThirdParty = "third_party"
)

type Config struct {
	ServiceType   string    `json:"service_type"` // "branpg" or "third_party"
	SyncInterval  int       `json:"sync_interval_minutes"`
	LastSyncHash  string    `json:"last_sync_hash"`
	LastSyncAt    time.Time `json:"last_sync_at"`
	
	// Branpg TODO OAuth
	BranpgAccessToken  string    `json:"branpg_access_token,omitempty"`
	BranpgRefreshToken string    `json:"branpg_refresh_token,omitempty"`
	BranpgTokenExpiry  time.Time `json:"branpg_token_expiry,omitempty"`
	BranpgUsername     string    `json:"branpg_username,omitempty"`
	BranpgConnectedAt  time.Time `json:"branpg_connected_at,omitempty"`
	
	// Third party
	ThirdPartyURL string `json:"third_party_url,omitempty"`
}

func DefaultConfig() *Config {
	return &Config{
		ServiceType:  ServiceTypeBranpg,
		SyncInterval: 10,
	}
}

func GetConfigPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	
	appDir := filepath.Join(configDir, "GalaxySyncService")
	if err := os.MkdirAll(appDir, 0755); err != nil {
		return "", err
	}
	
	return filepath.Join(appDir, "config.json"), nil
}

func Load() (*Config, error) {
	path, err := GetConfigPath()
	if err != nil {
		return DefaultConfig(), err
	}
	
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return DefaultConfig(), err
	}
	
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return DefaultConfig(), err
	}
	
	return &cfg, nil
}

func (c *Config) Save() error {
	path, err := GetConfigPath()
	if err != nil {
		return err
	}
	
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	
	return os.WriteFile(path, data, 0600)
}

func (c *Config) IsConnected() bool {
	if c.ServiceType == ServiceTypeBranpg {
		return c.BranpgAccessToken != "" && time.Now().Before(c.BranpgTokenExpiry)
	}
	return c.ThirdPartyURL != ""
}

func (c *Config) Disconnect() {
	c.BranpgAccessToken = ""
	c.BranpgRefreshToken = ""
	c.BranpgTokenExpiry = time.Time{}
	c.BranpgUsername = ""
	c.BranpgConnectedAt = time.Time{}
}
