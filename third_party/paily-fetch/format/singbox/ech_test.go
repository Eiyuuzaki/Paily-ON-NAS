package singbox

import (
	"bytes"
	"encoding/json"
	"encoding/pem"
	"reflect"
	"testing"

	"github.com/Eiyuuzaki/paily-on-nas/third_party/paily-fetch/node"
)

func TestGeneratePreservesExplicitECH(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		extra                    map[string]any
		present, enabled, pinned bool
	}{
		{"absent", nil, false, false, false},
		{"pinned", map[string]any{"ech-opts": map[string]any{"enable": true, "config": "+/8="}}, true, true, true},
		{"disabled", map[string]any{"ech-opts": map[string]any{"enable": false}, "ech": "+/8="}, true, false, false},
		{"disabled-pinned", map[string]any{"ech-opts": map[string]any{"enable": false, "config": "+/8="}}, true, false, true},
		{"dynamic", map[string]any{"ech-opts": map[string]any{"enable": true}}, true, true, false},
		{"malformed-pinned", map[string]any{"ech-opts": map[string]any{"enable": true, "config": "not base64!"}}, false, false, false},
		{"legacy-link", map[string]any{"ech": "+/8="}, true, true, true},
		{"legacy-singbox-lines", map[string]any{"ech": "-----BEGIN ECH CONFIGS-----,+/8=,-----END ECH CONFIGS-----"}, true, true, true},
		{"reality", map[string]any{"ech": "+/8=", "reality-opts": map[string]any{"public-key": "fixture"}}, false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := map[string]any{"name": tc.name, "type": "trojan", "server": "192.0.2.1", "port": 443, "password": "fixture", "tls": nil}
			for key, value := range tc.extra {
				raw[key] = value
			}
			n, err := node.New(raw)
			if err != nil {
				t.Fatal(err)
			}
			g := (&Factory{}).NewGenerator().(*Generator)
			g.Push(n, "")
			data, err := g.Generate(nil)
			if err != nil {
				t.Fatal(err)
			}
			var doc struct {
				Outbounds []struct {
					TLS struct {
						ECH *struct {
							Enabled bool
							Config  []string
						}
					}
				}
			}
			if err := json.Unmarshal(data, &doc); err != nil {
				t.Fatal(err)
			}
			if len(doc.Outbounds) != 1 {
				t.Fatalf("expected one outbound, got %d", len(doc.Outbounds))
			}
			ech := doc.Outbounds[0].TLS.ECH
			if (ech != nil) != tc.present {
				t.Fatalf("ECH = %#v; want present = %v", ech, tc.present)
			}
			if ech != nil {
				if ech.Enabled != tc.enabled || (len(ech.Config) > 0) != tc.pinned {
					t.Fatalf("unexpected ECH: %#v", ech)
				}
				if tc.pinned {
					if len(ech.Config) != 1 {
						t.Fatalf("expected a single PEM string, got %q", ech.Config)
					}
					block, rest := pem.Decode([]byte(ech.Config[0]))
					if block == nil || block.Type != "ECH CONFIGS" || !bytes.Equal(block.Bytes, []byte{0xfb, 0xff}) || len(rest) != 0 {
						t.Fatalf("incorrect ECH PEM: %q", ech.Config[0])
					}
				}
			}
			if !reflect.DeepEqual(n.ClashMap(), raw) {
				t.Fatal("modified source node")
			}
		})
	}
}
