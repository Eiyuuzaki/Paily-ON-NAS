package naming

import "testing"

func TestEntryRegionOf(t *testing.T) {
	cases := []struct{ raw, server, want string }{
		// Measured colo beats the (stale) name prefix.
		{`{"name":"CF-US-332","server":"104.16.146.186"}`, "", "HK"},
		// Raw carries IPv6; the IPv4 server column resolves the colo.
		{`{"name":"CF-US-14","server":"2605:52c0:2:7297:be24:11ff:feca:a702"}`, "104.25.240.0", "HK"},
		// Fallback to name prefix when the /24 was not measured.
		{`{"name":"CF-HKG-245","server":"203.0.113.7"}`, "", "HK"},
		{`{"name":"CF-JP-1","server":"203.0.113.7"}`, "", "JP"},
		// Non-CF node: no entry region.
		{`{"name":"plain-node","server":"104.16.146.186"}`, "104.16.146.186", ""},
	}
	for _, c := range cases {
		if got := entryRegionOf(c.raw, c.server); got != c.want {
			t.Errorf("entryRegionOf(%s)=%q want %q", c.raw, got, c.want)
		}
	}
}
