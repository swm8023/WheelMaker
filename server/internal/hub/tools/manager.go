package tools

import (
	"context"
	"encoding/json"
	"strings"

	rp "github.com/swm8023/wheelmaker/internal/protocol"
)

type ProjectInfo = rp.ProjectInfo

type ManagerConfig struct {
	HubID          string
	Projects       []ProjectInfo
	MonitorBaseDir string
	GlobalLockPath string
	HomeDir        string
}

type CommandError struct {
	Code    string
	Message string
}

func (e *CommandError) Error() string {
	if e == nil {
		return ""
	}
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

type Manager struct {
	cfg ManagerConfig

	npmCommand    *NPMCommand
	updateCommand *UpdateCommand
	skillsCommand *SkillsCommand
	tokenCommand  *TokenCommand
}

func NewManager(config ManagerConfig) *Manager {
	config.HubID = strings.TrimSpace(config.HubID)
	if config.HubID == "" {
		config.HubID = "wheelmaker-hub"
	}
	config.MonitorBaseDir = strings.TrimSpace(config.MonitorBaseDir)
	config.GlobalLockPath = strings.TrimSpace(config.GlobalLockPath)
	config.HomeDir = strings.TrimSpace(config.HomeDir)
	config.Projects = append([]ProjectInfo(nil), config.Projects...)
	return &Manager{
		cfg:           config,
		npmCommand:    NewNPMCommand(),
		updateCommand: NewUpdateCommand(config.MonitorBaseDir),
		skillsCommand: NewSkillsCommand(skillsCommandConfig{
			HubID:          config.HubID,
			Projects:       config.Projects,
			GlobalLockPath: config.GlobalLockPath,
			HomeDir:        config.HomeDir,
		}),
		tokenCommand: NewTokenCommand(),
	}
}

func (m *Manager) SetProjects(projects []ProjectInfo) {
	if m == nil {
		return
	}
	m.cfg.Projects = append([]ProjectInfo(nil), projects...)
	if m.skillsCommand != nil {
		m.skillsCommand.SetProjects(m.cfg.Projects)
	}
}

func (m *Manager) Handle(ctx context.Context, method string, payload json.RawMessage) (any, *CommandError) {
	if m == nil {
		return nil, &CommandError{Code: rp.CodeInternal, Message: "tools manager is not configured"}
	}
	switch strings.TrimSpace(method) {
	case "cmd.npm":
		if m.npmCommand == nil {
			m.npmCommand = NewNPMCommand()
		}
		out, err := m.npmCommand.Handle(ctx, payload)
		return out, npmErr(err)
	case "cmd.update":
		if m.updateCommand == nil {
			m.updateCommand = NewUpdateCommand(m.cfg.MonitorBaseDir)
		}
		out, err := m.updateCommand.Handle(ctx, payload)
		return out, updateErr(err)
	case "cmd.skills":
		if m.skillsCommand == nil {
			m.skillsCommand = NewSkillsCommand(skillsCommandConfig{
				HubID:          m.cfg.HubID,
				Projects:       m.cfg.Projects,
				GlobalLockPath: m.cfg.GlobalLockPath,
				HomeDir:        m.cfg.HomeDir,
			})
		}
		m.skillsCommand.SetProjects(m.cfg.Projects)
		out, err := m.skillsCommand.Handle(ctx, payload)
		return out, skillsErr(err)
	case "cmd.token":
		if m.tokenCommand == nil {
			m.tokenCommand = NewTokenCommand()
		}
		out, err := m.tokenCommand.Handle(ctx, payload)
		return out, tokenErr(err)
	default:
		return nil, &CommandError{Code: rp.CodeInvalidArgument, Message: "unsupported tools command"}
	}
}

func npmErr(err *npmCommandError) *CommandError {
	if err == nil {
		return nil
	}
	return &CommandError{Code: err.Code, Message: err.Message}
}

func updateErr(err *updateCommandError) *CommandError {
	if err == nil {
		return nil
	}
	return &CommandError{Code: err.Code, Message: err.Message}
}

func skillsErr(err *skillsCommandError) *CommandError {
	if err == nil {
		return nil
	}
	return &CommandError{Code: err.Code, Message: err.Message}
}

func tokenErr(err *tokenCommandError) *CommandError {
	if err == nil {
		return nil
	}
	return &CommandError{Code: err.Code, Message: err.Message}
}
