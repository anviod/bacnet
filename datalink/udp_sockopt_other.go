//go:build !windows

package datalink

import "syscall"

// setReuseUDP is a no-op outside Windows. On Unix we leave the socket exclusive
// (no SO_REUSEADDR) so a second bind to the same address/port fails.
func setReuseUDP(network, address string, c syscall.RawConn) error {
	return nil
}
