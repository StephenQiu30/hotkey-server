package webpush

import (
	"net"
	"testing"
)

func TestPublicPushIPRejectsLocalReservedAndDocumentationNetworks(t *testing.T) {
	for _, raw := range []string{
		"127.0.0.1", "10.0.0.1", "169.254.1.1", "100.64.0.1", "192.0.2.1", "198.51.100.1", "203.0.113.1",
		"224.0.0.1", "240.0.0.1", "::1", "fc00::1", "fe80::1",
	} {
		if publicPushIP(net.ParseIP(raw)) {
			t.Fatalf("publicPushIP(%q) = true", raw)
		}
	}
	for _, raw := range []string{"1.1.1.1", "8.8.8.8", "2606:4700:4700::1111"} {
		if !publicPushIP(net.ParseIP(raw)) {
			t.Fatalf("publicPushIP(%q) = false", raw)
		}
	}
}
