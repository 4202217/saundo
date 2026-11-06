package discovery

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
	"slices"
)

// MIB_IPNETROW layout in the MIB_IPNETTABLE buffer: dwIndex, dwPhysAddrLen,
// bPhysAddr[8], dwAddr, dwType. The table starts four bytes in, after the
// DWORD row count.
const (
	ipNetRowSize    = 24
	ipNetAddrOffset = 16
	ipNetTypeOffset = 20

	ipNetTypeDynamic = 3
	ipNetTypeStatic  = 4
)

var ipv4Broadcast = netip.AddrFrom4([4]byte{255, 255, 255, 255})

// parseIpNetTable decodes the buffer GetIpNetTable returns. Only live IPv4
// neighbors are candidates; multicast, broadcast, loopback, and unspecified
// addresses are never queried.
func parseIpNetTable(buf []byte) ([]string, error) {
	if len(buf) < 4 {
		return nil, errors.New("ipnet table: short header")
	}
	count := binary.LittleEndian.Uint32(buf[:4])
	if uint64(count) > uint64((len(buf)-4)/ipNetRowSize) {
		return nil, fmt.Errorf("ipnet table: %d rows do not fit %d bytes", count, len(buf))
	}
	seen := make(map[netip.Addr]struct{}, count)
	hosts := make([]string, 0, count)
	for i := uint32(0); i < count; i++ {
		off := 4 + int(i)*ipNetRowSize
		addr := binary.LittleEndian.Uint32(buf[off+ipNetAddrOffset:])
		typ := binary.LittleEndian.Uint32(buf[off+ipNetTypeOffset:])
		if typ != ipNetTypeDynamic && typ != ipNetTypeStatic {
			continue
		}
		ip := netip.AddrFrom4([4]byte{byte(addr), byte(addr >> 8), byte(addr >> 16), byte(addr >> 24)})
		if ip.IsUnspecified() || ip.IsMulticast() || ip.IsLoopback() || ip == ipv4Broadcast {
			continue
		}
		if _, ok := seen[ip]; ok {
			continue
		}
		seen[ip] = struct{}{}
		hosts = append(hosts, ip.String())
	}
	slices.Sort(hosts)
	return hosts, nil
}
