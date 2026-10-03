//go:build windows

package link

import (
	"errors"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

// adapterDescriptions uses the Windows IP Helper API rather than parsing
// localized PowerShell/netsh output. A user-facing alias like "Ethernet 3"
// can conceal the real "USB4 P2P Network Adapter" driver description.
func adapterDescriptions() map[int]string {
	result := make(map[int]string)
	size := uint32(15 * 1024)
	for tries := 0; tries < 3; tries++ {
		buf := make([]byte, size)
		first := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0]))
		err := windows.GetAdaptersAddresses(windows.AF_UNSPEC, 0, 0, first, &size)
		if err == nil {
			for entry := first; entry != nil; entry = entry.Next {
				if entry.Description == nil {
					continue
				}
				description := windows.UTF16PtrToString(entry.Description)
				result[int(entry.IfIndex)] = description
				if entry.Ipv6IfIndex != 0 {
					result[int(entry.Ipv6IfIndex)] = description
				}
			}
			runtime.KeepAlive(buf)
			return result
		}
		runtime.KeepAlive(buf)
		if !errors.Is(err, windows.ERROR_BUFFER_OVERFLOW) {
			break
		}
	}
	return result
}
