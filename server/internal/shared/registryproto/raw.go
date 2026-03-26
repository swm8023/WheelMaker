package registryproto

import "encoding/json"

func MustRaw(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
