package agentv2

import (
	"sort"
)

// Provider resolves launch details for one agent type.
type Provider interface {
	Name() string
	LaunchSpec() (exe string, args []string, env []string, err error)
}

func buildEnv(m map[string]string) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	env := make([]string, 0, len(keys))
	for _, k := range keys {
		env = append(env, k+"="+m[k])
	}
	return env
}
