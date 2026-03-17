package acp

// SessionConfigSnapshot is a compact view of session-level mode/model values.
type SessionConfigSnapshot struct {
	Mode  string
	Model string
}

func SessionConfigSnapshotFromOptions(opts []ConfigOption) SessionConfigSnapshot {
	snap := SessionConfigSnapshot{}
	for _, opt := range opts {
		if snap.Mode == "" && (opt.ID == "mode" || opt.Category == "mode") {
			snap.Mode = opt.CurrentValue
		}
		if snap.Model == "" && (opt.ID == "model" || opt.Category == "model") {
			snap.Model = opt.CurrentValue
		}
	}
	return snap
}
