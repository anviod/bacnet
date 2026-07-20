//go:build windows

package datalink

import "syscall"

// setReuseUDP enables SO_REUSEADDR and SO_BROADCAST for BACnet/IP communication.
// SO_REUSEADDR allows multiple BACnet entities (e.g., YABE and room-simulator)
// to share the same port on Windows, enabling broadcast Who-Is discovery.
// SO_BROADCAST is required for sending/receiving broadcast packets.
// The "Confirmed service not handled" issue is resolved by ensuring that
// ReadProperty requests are sent to the correct device's source port (from I-Am),
// not the broadcast port.
// 启用 SO_REUSEADDR 和 SO_BROADCAST 用于 BACnet/IP 通信。
// SO_REUSEADDR 允许多个 BACnet 实体（如 YABE 和 room-simulator）
// 在 Windows 上共享同一端口，实现广播 Who-Is 发现。
// SO_BROADCAST 是发送/接收广播包的必需选项。
// "Confirmed service not handled" 问题通过确保 ReadProperty 请求
// 发送到正确设备的源端口（来自 I-Am）而不是广播端口来解决。
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
