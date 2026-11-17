package distribution

import (
	"testing"

	"github.com/Eiyuuzaki/paily-on-nas/internal/cache"
)

func TestIsCFNodeOnRewrittenRaw(t *testing.T) {
	raw := `{"tls": true, "name": "CF-US-125", "port": 443, "type": "vless", "uuid": "2945b6ea-2ad7-40b1-8400-7e42e46f9770", "server": "108.162.198.89", "network": "ws", "ws-opts": {"path": "/?ed=2048", "headers": {"Host": "lencf.dpdns.org"}}, "security": "tls", "encryption": "none", "servername": "lencf.dpdns.org"}`
	if !isCFNode(raw) {
		t.Fatalf("isCFNode=false for rewritten raw")
	}
	e := &cache.NodeCacheEntry{Raw: raw, Server: "108.162.198.89", Region: "US", Protocol: "vless"}
	if got := egressIdentity(e); got != "srv:108.162.198.89" {
		t.Logf("identity (ProxyNode nil in test) = %s", got)
	}
	if isSkipCert(e) {
		t.Fatalf("skipcert should be false")
	}
	if isPlaintext(e) {
		t.Fatalf("tls:true => not plaintext")
	}
}
