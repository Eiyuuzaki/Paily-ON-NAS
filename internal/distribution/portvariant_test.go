package distribution

import (
	"testing"

	"github.com/Eiyuuzaki/paily-on-nas/internal/cache"
	node "github.com/Eiyuuzaki/paily-on-nas/third_party/paily-fetch/node"
)

func cfEntry(server, sni string, port int) *cache.NodeCacheEntry {
	m := map[string]any{"name": "n", "type": "vless", "server": server, "port": port}
	if sni != "" {
		m["servername"] = sni
	}
	n, err := node.New(m)
	if err != nil {
		panic(err)
	}
	return &cache.NodeCacheEntry{Server: server, ProxyNode: n}
}

// A self-built Worker dialled as IP+SNI is the only shape that gets a port twin.
func TestCFEntryPortSelfBuiltWorker(t *testing.T) {
	port, ok := cfEntryPort(cfEntry("104.16.1.1", "lencf.dpdns.org", 443), true)
	if !ok || port != 443 {
		t.Fatalf("want (443,true), got (%d,%v)", port, ok)
	}
}

// Free-list nodes reuse an IP server with a decoy SNI. They are not Workers,
// and nothing is listening on their second Cloudflare port.
func TestCFEntryPortIgnoresForeignNodes(t *testing.T) {
	if _, ok := cfEntryPort(cfEntry("141.94.16.4", "www.bing.com", 443), false); ok {
		t.Fatal("free-list node must not be duplicated onto another port")
	}
}

// Without a self-built origin the flag is false even for a real Worker shape.
func TestCFEntryPortRequiresSelfBuiltFlag(t *testing.T) {
	if _, ok := cfEntryPort(cfEntry("104.16.1.1", "lencf.dpdns.org", 443), false); ok {
		t.Fatal("self-built flag must gate the variant")
	}
}

// A domain server identifies its own service, so there is no assumed twin port.
func TestCFEntryPortRejectsDomainServer(t *testing.T) {
	if _, ok := cfEntryPort(cfEntry("etg.llieowell.workers.dev", "etg.llieowell.workers.dev", 443), true); ok {
		t.Fatal("domain server must not be duplicated")
	}
}

// No SNI means no shared Worker identity.
func TestCFEntryPortRejectsMissingSNI(t *testing.T) {
	if _, ok := cfEntryPort(cfEntry("104.16.1.1", "", 443), true); ok {
		t.Fatal("missing SNI must not be duplicated")
	}
}

// The clone must keep everything except the port: ech-opts and dialer-proxy are
// already baked into the map when variants are built, and dropping either would
// silently disable ECH or break the chain.
func TestWithPortPreservesFields(t *testing.T) {
	m := map[string]any{
		"name": "n", "type": "vless", "server": "104.16.1.1", "port": 443,
		"servername": "lencf.dpdns.org", "dialer-proxy": "entry",
		"ech-opts": map[string]any{"enable": true, "config": "AAAA"},
	}
	n, err := node.New(m)
	if err != nil {
		t.Fatal(err)
	}
	v := withPort(n, 8443)
	if v == nil {
		t.Fatal("withPort returned nil")
	}
	got := v.ClashMap()
	if got["port"] != 8443 {
		t.Fatalf("port = %v", got["port"])
	}
	if got["dialer-proxy"] != "entry" || got["servername"] != "lencf.dpdns.org" {
		t.Fatalf("lost chain/SNI fields: %v", got)
	}
	if _, ok := got["ech-opts"]; !ok {
		t.Fatal("lost ech-opts")
	}
	if orig := n.ClashMap()["port"]; orig != 443 {
		t.Fatalf("original mutated: port = %v", orig)
	}
}
