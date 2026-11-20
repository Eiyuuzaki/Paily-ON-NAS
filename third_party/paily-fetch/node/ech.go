package node

import (
	"encoding/base64"
	"encoding/pem"
	"strings"
)

// ECHOptions describes the explicit client ECH settings carried by a Clash map.
// An enabled option without Config requests the client's DNS-based lookup.
type ECHOptions struct {
	Enabled bool
	Config  []byte
}

// ExplicitECH reads source-supplied or caller-attached ECH; it never looks up
// DNS. Native ech-opts takes precedence over the legacy top-level ech string.
func ExplicitECH(clash map[string]any) (ECHOptions, bool) {
	if _, reality := clash["reality-opts"]; reality || clash["security"] == "reality" {
		return ECHOptions{}, false
	}
	if raw, present := clash["ech-opts"]; present {
		opts, ok := raw.(map[string]any)
		if !ok {
			return ECHOptions{}, false
		}
		enabled, ok := opts["enable"].(bool)
		if !ok {
			return ECHOptions{}, false
		}
		rawConfig := opts["config"]
		config, isString := rawConfig.(string)
		if pinned, ok := pinnedECH(config); ok {
			pinned.Enabled = enabled
			return pinned, true
		}
		// Malformed pinned input never becomes an enabled DNS lookup.
		return ECHOptions{Enabled: enabled}, !enabled || rawConfig == nil || (isString && config == "")
	}
	if config, ok := clash["ech"].(string); ok {
		return pinnedECH(config)
	}
	return ECHOptions{}, false
}

func pinnedECH(config string) (ECHOptions, bool) {
	config = strings.TrimSpace(config)
	if strings.HasPrefix(config, "-----BEGIN ECH CONFIGS-----") {
		// Older sing-box importers joined the PEM lines with commas.
		block, rest := pem.Decode([]byte(strings.ReplaceAll(config, ",", "\n")))
		if block != nil && block.Type == "ECH CONFIGS" && len(block.Bytes) > 0 && len(strings.TrimSpace(string(rest))) == 0 {
			return ECHOptions{Enabled: true, Config: block.Bytes}, true
		}
		return ECHOptions{}, false
	}
	for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if decoded, err := encoding.Strict().DecodeString(config); err == nil && len(decoded) > 0 {
			return ECHOptions{Enabled: true, Config: decoded}, true
		}
	}
	return ECHOptions{}, false
}
