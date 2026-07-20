//go:build windows

package datalink

import "syscall"

// setReuseUDP enables SO_REUSEADDR so the simulator can coexist with YABE on the
// same machine. It also enables SO_BROADCAST because Windows requires explicit
// broadcast permission on UDP sockets — without it, sending WhoIs broadcast
// packets silently fails and no BACnet devices are discovered.
// 同时启用 SO_BROADCAST，因为 Windows 要求 UDP 套接字显式允许广播 —
// 不设置此选项会导致 WhoIs 广播包静默失败，无法发现 BACnet 设备。
func setReuseUDP(network, address string, c syscall.RawConn) error {
	var opErr error
	err := c.Control(func(fd uintptr) {
		opErr = syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
		if opErr != nil {
			return
		}
		opErr = syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
	})
	if err != nil {
		return err
	}
	return opErr
}