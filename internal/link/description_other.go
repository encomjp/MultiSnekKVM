//go:build !windows

package link

// Other systems use the interface name; Windows additionally exposes
// a hardware description through the IP Helper API.
func adapterDescriptions() map[int]string { return map[int]string{} }
