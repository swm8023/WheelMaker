package config

// Config is the top-level config.json structure.
type Config struct {
	Projects []ProjectConfig `json:"projects"`
	Registry RegistryConfig  `json:"registry,omitempty"`
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
	AppID     string `json:"appID,omitempty"`
	AppSecret string `json:"appSecret,omitempty"`
}

// ClientConf describes the AI agent side for a project.
type ClientConf struct {
	Agent string `json:"agent,omitempty"`
}

// RegistryConfig configures registry sync independent of IM mode.
type RegistryConfig struct {
	// Port is the TCP port used by local listen or remote connect target.
	Port int `json:"port,omitempty"`
	// Listen controls mode:
	// true = start local registry server and report to it;
	// false = connect to remote registry server.
	Listen bool `json:"listen,omitempty"`
	// Server is host/address for listen or connect.
	// In listen mode default is 127.0.0.1 when empty.
	Server string `json:"server,omitempty"`
	// Token is optional shared secret for registry auth.
	Token string `json:"token,omitempty"`
	// HubID is optional stable hub identity. Empty falls back to hostname.
	HubID string `json:"hubId,omitempty"`
}
