package clash

import (
	"encoding/json"

	"github.com/Eiyuuzaki/paily-on-nas/third_party/paily-fetch/internal/format"
)

// Config holds generation options for the clash output format.
// When Template is non-empty it must be a complete Clash YAML configuration.
// The generator will:
//   - replace the top-level "proxies" list with all pushed proxy nodes, and
//   - append every pushed node's name to the "proxies" list of every proxy-group.
//
// When Template is empty the generator emits a plain {"proxies": [...]} document.
type Config struct {
	Template string `json:"template,omitempty"`
}

// parseConfig deserialises a FormatConfig JSON blob into a Config.
// A nil or empty blob returns a zero-value Config (no template). A non-empty
// blob that does not decode is an error: falling back to a zero-value Config
// would drop the stored template and serve a bare document without its
// proxy-groups and rules, and the subscription itself would still look healthy
// to the operator.
func parseConfig(raw format.FormatConfig) (Config, error) {
	if len(raw) == 0 {
		return Config{}, nil
	}
	var c Config
	if err := json.Unmarshal(raw, &c); err != nil {
		return Config{}, err
	}
	return c, nil
}
