//go:build !windows

package datalink

import "syscall"

// setReuseUDP enables SO_BROADCAST on Unix to allow sending broadcast WhoIs
// packets. Unlike Windows, SO_REUSEADDR is not set — leaving the socket
// exclusive prevents a second bind to the same address/port from succeeding.
// On Linux, SO_BROADCAST is not enabled by default on UDP sockets; without it,
// WriteTo with a broadcast destination address returns EACCES and WhoIs
// broadcast packets silently fail.
// 在 Unix/Linux 上启用 SO_BROADCAST，允许发送广播 WhoIs 包。
// 与 Windows 不同，不设置 SO_REUSEADDR，保持排他性绑定。
func setReuseUDP(network, address string, c syscall.RawConn) error {
	var opErr error
	err := c.Control(func(fd uintptr) {
		opErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
	})
	if err != nil {
		return err
	}
	return opErr
}