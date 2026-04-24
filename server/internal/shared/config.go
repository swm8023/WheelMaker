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
	Version  json.RawMessage    `json:"version,omitempty"`
	Projects []rawProjectConfig `json:"projects"`
}

type rawProjectConfig struct {
	Debug  json.RawMessage `json:"debug,omitempty"`
	IM     json.RawMessage `json:"im,omitempty"`
	Client json.RawMessage `json:"client,omitempty"`
}

type rawIMConfig struct {
	Version json.RawMessage `json:"version,omitempty"`
}

// LogConfig controls the operational log system.
type LogConfig struct {
	// Level is the minimum log level to emit: "debug", "info", "warn" (default), "error".
	Level string `json:"level,omitempty"`
}

// ProjectConfig describes one WheelMaker project.
type ProjectConfig struct {
	Name     string        `json:"name"`
	Path     string        `json:"path"`
	Feishu   *FeishuConfig `json:"feishu,omitempty"`
	IMFilter IMFilterConf  `json:"imFilter,omitempty"`
}

// IMFilterConf controls which IM-visible events the IM adapter suppresses.
type IMFilterConf struct {
	// Block contains IM-level event types to suppress.
	// Supported values depend on the channel implementation and commonly include:
	// thought, tool/tool_call, text, system, plan, config_option_update,
	// available_commands_update, done/prompt_result.
	Block []string `json:"block,omitempty"`
}

// FeishuConfig describes Feishu transport config.
type FeishuConfig struct {
	AppID     string `json:"app_id,omitempty"`
	AppSecret string `json:"app_secret,omitempty"`
}

type rawFeishuConfig struct {
	AppIDLegacy     string `json:"appID,omitempty"`
	AppSecretLegacy string `json:"appSecret,omitempty"`
	AppIDSnake      string `json:"app_id,omitempty"`
	AppSecretSnake  string `json:"app_secret,omitempty"`
	AppSecretTypo   string `json:"app_secrect,omitempty"`
}

func (c *FeishuConfig) UnmarshalJSON(data []byte) error {
	var raw rawFeishuConfig
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	c.AppID = firstNonEmpty(raw.AppIDSnake, raw.AppIDLegacy)
	c.AppSecret = firstNonEmpty(raw.AppSecretSnake, raw.AppSecretTypo, raw.AppSecretLegacy)
	return nil
}

func (p ProjectConfig) IMType() string {
	if p.HasFeishu() {
		return "feishu"
	}
	return "app"
}

func (p ProjectConfig) HasFeishu() bool {
	return p.Feishu != nil && strings.TrimSpace(p.Feishu.AppID) != "" && strings.TrimSpace(p.Feishu.AppSecret) != ""
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

	if err := validateRemovedLegacyFields(path, data); err != nil {
		return nil, err
	}

	var cfg AppConfig
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	if err := validateFeishuConfig(path, cfg.Projects); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func validateRemovedLegacyFields(path string, data []byte) error {
	var raw rawAppConfig
	if err := json.Unmarshal(data, &raw); err != nil {
		// Let strict decode in LoadConfig return the canonical parse error.
		return nil
	}

	if len(raw.Version) != 0 {
		return fmt.Errorf("parse config %s: im.version has been removed; IM is the only supported runtime", path)
	}

	for _, project := range raw.Projects {
		if len(project.Debug) != 0 {
			return fmt.Errorf("parse config %s: projects[].debug has been removed", path)
		}
		if len(project.IM) != 0 {
			var legacyIM rawIMConfig
			if err := json.Unmarshal(project.IM, &legacyIM); err == nil && len(legacyIM.Version) != 0 {
				return fmt.Errorf("parse config %s: im.version has been removed; IM is the only supported runtime", path)
			}
			return fmt.Errorf("parse config %s: projects[].im has been removed; use projects[].feishu instead", path)
		}
		if len(project.Client) != 0 {
			return fmt.Errorf("parse config %s: projects[].client has been removed; provider is auto-detected", path)
		}
	}
	return nil
}

func validateFeishuConfig(path string, projects []ProjectConfig) error {
	for _, project := range projects {
		if project.Feishu == nil {
			continue
		}
		appID := strings.TrimSpace(project.Feishu.AppID)
		appSecret := strings.TrimSpace(project.Feishu.AppSecret)
		if appID == "" || appSecret == "" {
			return fmt.Errorf("parse config %s: projects[].feishu requires both app_id and app_secret", path)
		}
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
