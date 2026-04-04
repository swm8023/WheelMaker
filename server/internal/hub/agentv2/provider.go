package agentv2

import (
	"sort"
)

// ACPProvider resolves launch details for one ACP agent type.
type ACPProvider interface {
	Name() string
	LaunchSpec() (exe string, args []string, env []string, err error)
}

// Provider is kept as a compatibility alias for ACPProvider.
type Provider = ACPProvider

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
