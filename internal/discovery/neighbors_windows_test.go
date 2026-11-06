//go:build windows

package discovery

import (
	"net"
	"testing"
)

// TestNeighborHostsReadsTheArpTable exercises the iphlpapi binding. It asserts
// only that the table decodes into IPv4 strings; the neighbor set is whatever
// this machine has.
func TestNeighborHostsReadsTheArpTable(t *testing.T) {
	hosts, err := neighborHosts()
	if err != nil {
		t.Fatal(err)
	}
	for _, host := range hosts {
		if ip := net.ParseIP(host); ip == nil || ip.To4() == nil {
			t.Fatalf("neighbor %q is not an IPv4 address", host)
		}
	}
	t.Logf("neighbors: %v", hosts)
}
