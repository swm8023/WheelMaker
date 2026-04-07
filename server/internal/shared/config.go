package shared

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// AppConfig is the top-level config.json structure.
type AppConfig struct {
	Projects []ProjectConfig `json:"projects"`
	Registry RegistryConfig  `json:"registry,omitempty"`
	Monitor  MonitorConfig   `json:"monitor,omitempty"`
	Log      LogConfig       `json:"log,omitempty"`
}

type rawAppConfig struct {
	Projects []rawProjectConfig `json:"projects"`
}

type rawProjectConfig struct {
	Debug json.RawMessage `json:"debug,omitempty"`
}

// LogConfig controls the operational log system.
type LogConfig struct {
	// Level is the minimum log level to emit: "debug", "info", "warn" (default), "error".
	Level string `json:"level,omitempty"`
}

// ProjectConfig describes one WheelMaker project.
type ProjectConfig struct {
	Name   string     `json:"name"`
	Path   string     `json:"path"`
	YOLO   bool       `json:"yolo,omitempty"`
	IM     IMConfig   `json:"im"`
	Client ClientConf `json:"client"`
}

// IMConfig describes the IM transport for a project.
type IMConfig struct {
	Type      string `json:"type"`
	AppID     string `json:"appID,omitempty"`
	AppSecret string `json:"appSecret,omitempty"`
}

// ClientConf describes the AI agent side for a project.
type ClientConf struct {
	Agent    string       `json:"agent,omitempty"`
	IMFilter IMFilterConf `json:"imFilter,omitempty"`
}

// IMFilterConf controls which IM-visible events the IM adapter suppresses.
type IMFilterConf struct {
	// Block contains IM-level event types to suppress.
	// Supported values depend on the channel implementation and commonly include:
	// thought, tool/tool_call, text, system, plan, config_option_update,
	// available_commands_update, done/prompt_result.
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
	if bytes.Contains(data, []byte(`"version"`)) {
		return nil, fmt.Errorf("parse config %s: im.version has been removed; IM is the only supported runtime", path)
	}
	var raw rawAppConfig
	if err := json.Unmarshal(data, &raw); err == nil {
		for _, project := range raw.Projects {
			if len(project.Debug) != 0 {
				return nil, fmt.Errorf("parse config %s: projects[].debug has been removed", path)
			}
		}
	}
	var cfg AppConfig
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	for _, project := range cfg.Projects {
		switch strings.ToLower(strings.TrimSpace(project.IM.Type)) {
		case "feishu", "app":
		default:
			return nil, fmt.Errorf("parse config %s: unsupported im.type %q (supported: feishu, app)", path, project.IM.Type)
		}
	}
	return &cfg, nil
}
