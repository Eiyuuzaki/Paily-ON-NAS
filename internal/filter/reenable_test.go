package filter

import "testing"

// Proposed re-enable filter: allow http nodes whose raw carries tls:true
// (self-signed HTTPS proxies), keep plaintext http and all socks5 excluded.
func TestReEnableHTTPTLSFilter(t *testing.T) {
	expr := `Region == "UN" || Latency > 5000 || Protocol == "socks5" || (Protocol == "http" && RawConfig["tls"] != true)`

	cases := []struct {
		name   string
		env    NodeFilterEnv
		expect bool // true = excluded (filtered out)
	}{
		{"https-proxy alive", NodeFilterEnv{Protocol: "http", Region: "US", Latency: 500, RawConfig: map[string]any{"tls": true}}, false},
		{"plaintext http", NodeFilterEnv{Protocol: "http", Region: "US", Latency: 500, RawConfig: map[string]any{}}, true},
		{"http no tls field", NodeFilterEnv{Protocol: "http", Region: "US", Latency: 500}, true},
		{"socks5 even with tls", NodeFilterEnv{Protocol: "socks5", Region: "US", Latency: 500, RawConfig: map[string]any{"tls": true}}, true},
		{"UN region", NodeFilterEnv{Protocol: "vless", Region: "UN", Latency: 500}, true},
		{"too slow", NodeFilterEnv{Protocol: "vless", Region: "US", Latency: 6000}, true},
		{"normal vless passes", NodeFilterEnv{Protocol: "vless", Region: "SG", Latency: 800}, false},
	}
	for _, c := range cases {
		excluded, err := EvalBool(expr, c.env)
		if err != nil {
			t.Errorf("%s: EvalBool error: %v", c.name, err)
			continue
		}
		if excluded != c.expect {
			t.Errorf("%s: excluded=%v, want %v", c.name, excluded, c.expect)
		}
	}
}
