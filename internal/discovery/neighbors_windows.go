//go:build windows

package discovery

import (
	"fmt"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procGetIpNetTable = windows.NewLazySystemDLL("iphlpapi.dll").NewProc("GetIpNetTable")

// neighborHosts returns the IPv4 addresses in the Windows ARP neighbor table.
// The table is the only way to find a receiver that does not answer multicast
// but does answer a directed DNS-SD query; the call is local and sends no
// packets.
func neighborHosts() ([]string, error) {
	var size uint32
	ret, _, _ := procGetIpNetTable.Call(0, uintptr(unsafe.Pointer(&size)), 0)
	if ret != 0 && windows.Errno(ret) != windows.ERROR_INSUFFICIENT_BUFFER {
		return nil, fmt.Errorf("discovery: GetIpNetTable size: %w", windows.Errno(ret))
	}
	if size == 0 {
		return nil, nil
	}
	// The table can grow between the size probe and the read; retry with the
	// size the API reports.
	for attempt := 0; attempt < 3; attempt++ {
		buf := make([]byte, size)
		ret, _, _ = procGetIpNetTable.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), 0)
		runtime.KeepAlive(buf)
		if ret == 0 {
			return parseIpNetTable(buf)
		}
		if windows.Errno(ret) != windows.ERROR_INSUFFICIENT_BUFFER {
			return nil, fmt.Errorf("discovery: GetIpNetTable: %w", windows.Errno(ret))
		}
	}
	return nil, fmt.Errorf("discovery: GetIpNetTable kept growing")
}
