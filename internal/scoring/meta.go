package scoring

import "encoding/json"

// parsePingMeta pulls the probe reliability counters the checker stores in a
// deep check's metadata. Missing values are treated as an all-success sample so a
// node is never penalised for the absence of metadata.
func parsePingMeta(raw string) (success, total int) {
	if raw == "" || raw == "{}" {
		return 0, 0
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return 0, 0
	}
	get := func(k string) int {
		if v, ok := m[k].(float64); ok {
			return int(v)
		}
		return -1
	}
	s, t := get("ping_success"), get("ping_total")
	if s < 0 || t <= 0 {
		return 0, 0
	}
	return s, t
}
