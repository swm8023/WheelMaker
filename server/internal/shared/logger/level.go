package logger

// Level represents minimum log severity.
type Level int

const (
	// LevelDebug enables debug sink and all operational levels.
	LevelDebug Level = iota
	// LevelInfo emits informational, warning, and error messages.
	LevelInfo
	// LevelWarn emits warnings and errors (default).
	LevelWarn
	// LevelError emits errors only.
	LevelError
)

// ParseLevel converts a string to Level. Unknown values default to LevelWarn.
func ParseLevel(s string) Level {
	switch s {
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "error":
		return LevelError
	default:
		return LevelWarn
	}
}

// Config holds logger setup parameters.
type Config struct {
	// Level is the minimum severity to emit. Default LevelWarn.
	Level Level
	// LogFile appends operational logs (Info/Warn/Error) in addition to stderr.
	LogFile string
	// DebugLogFile stores Debug and protocol-trace output.
	// It is truncated at startup when debug is enabled.
	DebugLogFile string
}
