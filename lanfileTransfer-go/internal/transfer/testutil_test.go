package transfer

import (
	"net"
)

// receiverFixture listens on loopback and accepts a single connection, handing it
// to handler on a background goroutine. addr is what TransferManager.executeSend
// dials, since the manager opens its own connection.
type receiverFixture struct {
	addr string
	ln   net.Listener
}

func startReceiver(handler func(net.Conn)) *receiverFixture {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil
	}

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		handler(conn)
	}()

	return &receiverFixture{addr: ln.Addr().String(), ln: ln}
}

func (f *receiverFixture) close() {
	if f != nil && f.ln != nil {
		f.ln.Close()
	}
}
