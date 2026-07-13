//go:build windows

package datalink

import "syscall"

// setReuseUDP enables SO_REUSEADDR so the simulator can coexist with YABE on the
// same machine. When room-simulator is bound to a specific IP (e.g. 192.168.3.115)
// and YABE is bound to 0.0.0.0:47808, Windows delivers unicast ReadProperty
// requests addressed to that specific IP to the room-simulator socket, while
// broadcast Who-Is messages are received by both applications.
func setReuseUDP(network, address string, c syscall.RawConn) error {
	var opErr error
	err := c.Control(func(fd uintptr) {
		opErr = syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
	})
	if err != nil {
		return err
	}
	return opErr
}
