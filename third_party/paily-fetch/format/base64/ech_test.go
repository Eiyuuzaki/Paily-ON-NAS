package base64fmt

import (
	"encoding/base64"
	"net/url"
	"strings"
	"testing"

	"github.com/Eiyuuzaki/paily-on-nas/third_party/paily-fetch/node"
)

func TestGeneratePreservesExplicitECHInLinks(t *testing.T) {
	for _, ptype := range []string{"vless", "trojan"} {
		for _, tc := range []struct {
			name  string
			extra map[string]any
			want  string
		}{
			{"absent", nil, ""},
			{"pinned", map[string]any{"ech-opts": map[string]any{"enable": true, "config": "+/8="}}, "+/8="},
			{"legacy-link", map[string]any{"ech": "+/8="}, "+/8="},
			{"legacy-pem", map[string]any{"ech": "-----BEGIN ECH CONFIGS-----,+/8=,-----END ECH CONFIGS-----"}, "+/8="},
			{"disabled", map[string]any{"ech-opts": map[string]any{"enable": false}, "ech": "+/8="}, ""},
			{"reality", map[string]any{"ech": "+/8=", "reality-opts": map[string]any{"public-key": "fixture"}}, ""},
		} {
			t.Run(ptype+"/"+tc.name, func(t *testing.T) {
				raw := map[string]any{"name": tc.name, "type": ptype, "server": "192.0.2.1", "port": 443, "uuid": "00000000-0000-0000-0000-000000000001", "password": "fixture", "tls": true}
				for key, value := range tc.extra {
					raw[key] = value
				}
				n, err := node.New(raw)
				if err != nil {
					t.Fatal(err)
				}
				g := (&factory{}).NewGenerator()
				g.Push(n, "")
				out, err := g.Generate(nil)
				if err != nil {
					t.Fatal(err)
				}
				decoded, err := base64.StdEncoding.DecodeString(string(out))
				if err != nil {
					t.Fatal(err)
				}
				u, err := url.Parse(strings.TrimSpace(string(decoded)))
				if err != nil {
					t.Fatal(err)
				}
				q, err := url.ParseQuery(u.RawQuery)
				if err != nil {
					t.Fatal(err)
				}
				if got := q.Get("ech"); got != tc.want || q.Has("ech") != (tc.want != "") {
					t.Fatalf("ECH = %q, want %q", got, tc.want)
				}
			})
		}
	}
}
