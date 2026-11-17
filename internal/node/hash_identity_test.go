package node

import "testing"

// The rotating-hostname scheme observed from ikuuu (2026-11-18): the same
// backend is republished under a new random label on every request.
func TestHashIdentityServerCollapsesSameBackend(t *testing.T) {
	// Same backend (jp06-jz-vm0) seen with three different rotating labels.
	same := []string{
		"15ofrbw-g03.jp06-jz-vm0.entry.fr0528.art",
		"7phwdel-g03.jp06-jz-vm0.entry.fr0528.art",
		"n6wzrbw-g03.jp06-jz-vm0.entry.fr0528.art",
	}
	want := "g03.jp06-jz-vm0.entry.fr0528.art"
	for _, in := range same {
		if got := hashIdentityServer(in); got != want {
			t.Errorf("hashIdentityServer(%q) = %q, want %q", in, got, want)
		}
	}
}

// Different backends must stay different even after normalization.
func TestHashIdentityServerKeepsBackendsSeparate(t *testing.T) {
	a := hashIdentityServer("15ofrbw-g03.jp06-jz-vm0.entry.fr0528.art")
	b := hashIdentityServer("npwwebx-g03.jp04-1d-vm0.entry.fr0528.art")
	if a == b {
		t.Fatalf("distinct backends collapsed to %q", a)
	}
}

// Everything else must keep its identity: IPs, ordinary hostnames, and labels
// that are not a plausible rotating token.
func TestHashIdentityServerLeavesOthersAlone(t *testing.T) {
	untouched := []string{
		"1.2.3.4",
		"104.17.169.1",
		"2406:da18:1ec6:2e00:7626:1f1e:79d8:cd16",
		"lencf.dpdns.org",
		"04535dt.us8.h1clayd.click",
		"88f7008b-tmm5c0-tonrqx-1khwf.nw4.hyhuawei.example",
		"verylongprefix-g03.jp01.entry.example", // label too long
		"short-g03.jp01.entry.example",          // label too short
		"-g03.jp01.entry.example",               // empty label
		"ABC1234-g03.jp01.entry.example",        // uppercase is not a rotating token
	}
	for _, in := range untouched {
		if got := hashIdentityServer(in); got != in {
			t.Errorf("hashIdentityServer(%q) = %q, want unchanged", in, got)
		}
	}
}
