package clash

import (
	"reflect"
	"testing"

	"github.com/Eiyuuzaki/paily-on-nas/third_party/paily-fetch/node"
	"gopkg.in/yaml.v3"
)

func TestGeneratePreservesExplicitECH(t *testing.T) {
	for _, tc := range []struct {
		name  string
		extra map[string]any
		want  map[string]any
	}{
		{"absent", nil, nil},
		{"clash-pinned", map[string]any{"ech-opts": map[string]any{"enable": true, "config": "+/8=", "query-server-name": "public.example"}}, map[string]any{"enable": true, "config": "+/8=", "query-server-name": "public.example"}},
		{"clash-disabled", map[string]any{"ech-opts": map[string]any{"enable": false}, "ech": "+/8="}, map[string]any{"enable": false}},
		{"link-pinned", map[string]any{"ech": "+/8="}, map[string]any{"enable": true, "config": "+/8="}},
		{"reality-with-link-ech", map[string]any{"ech": "+/8=", "reality-opts": map[string]any{"public-key": "fixture"}}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := map[string]any{"name": tc.name, "type": "vless", "server": "192.0.2.1", "tls": true}
			for key, value := range tc.extra {
				raw[key] = value
			}
			n, err := node.New(raw)
			if err != nil {
				t.Fatal(err)
			}
			g := (&Factory{}).NewGenerator().(*Generator)
			g.SetECHEnabled(false)
			g.Push(n, "")
			out, err := g.Generate(nil)
			if err != nil {
				t.Fatal(err)
			}
			var doc struct {
				Proxies []map[string]any `yaml:"proxies"`
			}
			if err := yaml.Unmarshal(out, &doc); err != nil {
				t.Fatal(err)
			}
			if len(doc.Proxies) != 1 {
				t.Fatalf("expected one proxy, got %d", len(doc.Proxies))
			}
			got, exists := doc.Proxies[0]["ech-opts"].(map[string]any)
			if exists != (tc.want != nil) || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("ECH = %#v, want %#v", got, tc.want)
			}
			if !reflect.DeepEqual(n.ClashMap(), raw) {
				t.Fatal("modified source node")
			}
		})
	}
}
