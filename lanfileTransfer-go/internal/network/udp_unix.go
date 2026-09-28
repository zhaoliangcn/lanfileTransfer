//go:build !windows

package network

import (
	"net"
	"syscall"
)

func setBroadcastOptions(conn *net.UDPConn) error {
	sysConn, err := conn.SyscallConn()
	if err != nil {
		return err
	}

	var controlErr error
	if err := sysConn.Control(func(fd uintptr) {
		if err := syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1); err != nil {
			controlErr = err
			return
		}
		if err := syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1); err != nil {
			controlErr = err
			return
		}
	}); err != nil {
		return err
	}

	return controlErr
}
