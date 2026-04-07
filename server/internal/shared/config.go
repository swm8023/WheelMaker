package shared

import (
	"encoding/json"
	"fmt"
	"os"
)

// AppConfig is the top-level config.json structure.
type AppConfig struct {
	Projects []ProjectConfig `json:"projects"`
	Registry RegistryConfig  `json:"registry,omitempty"`
	Monitor  MonitorConfig   `json:"monitor,omitempty"`
	Log      LogConfig       `json:"log,omitempty"`
}

// LogConfig controls the operational log system.
type LogConfig struct {
	// Level is the minimum log level to emit: "debug", "info", "warn" (default), "error".
	Level string `json:"level,omitempty"`
}

// ProjectConfig describes one WheelMaker project.
type ProjectConfig struct {
	Name   string     `json:"name"`
	Debug  bool       `json:"debug,omitempty"`
	Path   string     `json:"path"`
	YOLO   bool       `json:"yolo,omitempty"`
	IM     IMConfig   `json:"im"`
	Client ClientConf `json:"client"`
}

// IMConfig describes the IM transport for a project.
type IMConfig struct {
	Type      string `json:"type"`
	Version   int    `json:"version,omitempty"`
	AppID     string `json:"appID,omitempty"`
	AppSecret string `json:"appSecret,omitempty"`
}

// ClientConf describes the AI agent side for a project.
type ClientConf struct {
	Agent    string       `json:"agent,omitempty"`
	IMFilter IMFilterConf `json:"imFilter,omitempty"`
}

// IMFilterConf controls which client updates are blocked from IM delivery.
type IMFilterConf struct {
	// Block contains update types to suppress from IM output.
	// Supported values include: thought, tool/tool_call, text, system, plan,
	// config_option_update, available_commands_update, done, error.
	Block []string `json:"block,omitempty"`
}

// MonitorConfig configures the wheelmaker-monitor web dashboard.
type MonitorConfig struct {
	Port int `json:"port,omitempty"` // HTTP listen port (default: 9631)
}

// RegistryConfig configures registry sync independent of IM mode.
type RegistryConfig struct {
	Port   int    `json:"port,omitempty"`
	Listen bool   `json:"listen,omitempty"`
	Server string `json:"server,omitempty"`
	Token  string `json:"token,omitempty"`
	HubID  string `json:"hubId,omitempty"`
}

// LoadConfig reads and parses the config file at path.
func LoadConfig(path string) (*AppConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config file not found at %s", path)
		}
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	var cfg AppConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	return &cfg, nil
}
