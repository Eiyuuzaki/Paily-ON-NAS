package cache

import (
	"testing"

	"github.com/Eiyuuzaki/paily-on-nas/internal/db/model"
)

// A domestic plaintext node WITH credentials may serve as a chain entry: the
// entry hop only relays an already-encrypted tunnel, so it never sees content.
func TestChainEntryCandidate_CNWithCredentials(t *testing.T) {
	n := &model.Node{Region: "CN", Protocol: "socks5"}
	for _, key := range []string{"uuid", "username", "password"} {
		raw := map[string]any{key: "cred-value"}
		if !chainEntryCandidate(n, raw) {
			t.Errorf("CN socks5 with %s should qualify as chain entry", key)
		}
	}
}

// Without credentials anyone could impersonate the relay and hijack the
// tunnel, so the node must not be kept.
func TestChainEntryCandidate_NoCredentials(t *testing.T) {
	n := &model.Node{Region: "CN", Protocol: "http"}
	if chainEntryCandidate(n, map[string]any{"server": "test.example.invalid"}) {
		t.Error("credential-less node must not qualify")
	}
	if chainEntryCandidate(n, nil) {
		t.Error("nil raw map must not qualify")
	}
	if chainEntryCandidate(n, map[string]any{"uuid": ""}) {
		t.Error("empty credential must not qualify")
	}
}

// The entry allowance is scoped to domestic nodes: an overseas plaintext node
// gains nothing from being a relay and stays excluded.
func TestChainEntryCandidate_NonCNRejected(t *testing.T) {
	for _, region := range []string{"US", "DE", "NL", ""} {
		n := &model.Node{Region: region, Protocol: "socks5"}
		if chainEntryCandidate(n, map[string]any{"uuid": "cred"}) {
			t.Errorf("region %q must not qualify as chain entry", region)
		}
	}
}

// Only relay-capable protocols qualify; a CN vless node is already handled by
// the normal exit path and needs no special casing.
func TestChainEntryCandidate_ProtocolScope(t *testing.T) {
	raw := map[string]any{"uuid": "cred"}
	for _, p := range []string{"vless", "vmess", "trojan", "ss"} {
		n := &model.Node{Region: "CN", Protocol: p}
		if chainEntryCandidate(n, raw) {
			t.Errorf("protocol %q should not use the entry-only path", p)
		}
	}
}