package node

import "testing"

func TestIsUnroutableServer(t *testing.T) {
	blocked := []string{
		"127.0.0.1", "127.0.0.10", "0.0.0.0", "10.1.2.3", "192.168.1.1",
		"172.16.0.1", "172.31.255.255", "169.254.1.1", "100.64.0.1", "100.127.255.254",
		"::1", "fe80::1", "fd00::1",
	}
	for _, s := range blocked {
		if !isUnroutableServer(s) {
			t.Errorf("%s should be unroutable", s)
		}
	}
	allowed := []string{
		"104.21.22.222", "172.66.129.76", "172.67.207.38", "1.1.1.1",
		"example.com", "lencf.dpdns.org", "2606:4700:3036::6815:16de", "",
		"100.63.255.255", "100.128.0.1",
	}
	for _, s := range allowed {
		if isUnroutableServer(s) {
			t.Errorf("%s should be routable", s)
		}
	}
}
