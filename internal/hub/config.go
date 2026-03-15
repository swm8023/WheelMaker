package hub

import (
	"encoding/json"
	"fmt"
	"os"
)

// Config is the top-level config.json structure.
type Config struct {
	Projects []ProjectConfig `json:"projects"`
	Feishu   FeishuConfig    `json:"feishu,omitempty"`
}

// ProjectConfig describes one WheelMaker project.
type ProjectConfig struct {
	Name   string     `json:"name"`
	IM     IMConfig   `json:"im"`
	Client ClientConf `json:"client"`
}

// IMConfig describes the IM transport for a project.
type IMConfig struct {
	Type      string `json:"type"`
	AppID     string `json:"appID,omitempty"`
	AppSecret string `json:"appSecret,omitempty"`
	Debug     bool   `json:"debug,omitempty"`
}

// ClientConf describes the agent side for a project.
type ClientConf struct {
	Adapter string `json:"adapter"`
	Path    string `json:"path"`
}

// FeishuConfig holds shared Feishu settings used across all feishu-type projects.
type FeishuConfig struct {
	VerificationToken string `json:"verificationToken,omitempty"`
}

// LoadConfig reads and parses the config file at path.
// Returns a clear error if the file is missing or malformed.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config file not found at %s", path)
		}
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	return &cfg, nil
}
