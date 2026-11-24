package singbox

import (
	"encoding/json"

	"github.com/Eiyuuzaki/paily-on-nas/third_party/paily-fetch/internal/format"
)

// Config holds generation options for the sing-box output format.
// When Template is non-empty it must be a complete sing-box JSON configuration.
// The generator will:
//   - keep all management outbounds from the template (selector, urltest, direct,
//     dns, block, tproxy, redirect, tun, sniffer),
//   - discard any proxy outbounds already in the template (they are replaced),
//   - append every pushed proxy tag to the "outbounds" list of every selector and
//     urltest outbound, and
//   - assemble the final outbounds array as:
//     [selector/urltest outbounds] + [pushed proxy outbounds] + [direct/other infra].
//
// When Template is empty the generator emits a plain {"outbounds": [...]} document.
type Config struct {
	Template string `json:"template,omitempty"`
}

// parseConfig deserialises a FormatConfig JSON blob into a Config.
// A nil or empty blob returns a zero-value Config (no template). A non-empty
// blob that does not decode is an error: falling back to a zero-value Config
// would drop the stored template and emit a bare document without its
// selectors, routing rules and dns block, and the subscription itself would
// still look healthy to the operator.
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

// managementTypes are sing-box outbound types that represent routing/management
// constructs. These are preserved from the template; everything else is replaced
// by the pushed proxy outbounds.
var managementTypes = map[string]bool{
	"selector": true,
	"urltest":  true,
	"direct":   true,
	"dns":      true,
	"block":    true,
	"tproxy":   true,
	"redirect": true,
	"tun":      true,
	"sniffer":  true,
}
