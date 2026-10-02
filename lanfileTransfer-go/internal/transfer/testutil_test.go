package transfer

import (
	"net"

	"LanFileTransfer-Go/internal/network"
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

// chunkRecord is one wire-level chunk as the receiver saw it.
type chunkRecord struct {
	index int64
	data  []byte
}

// drainReceiver collects every packet on the connection until the sender hangs
// up, then publishes them. accepted reports whether a connection ever arrived,
// and both channels are buffered so the handler never blocks on an idle test.
func drainReceiver() (*receiverFixture, chan []chunkRecord, chan struct{}) {
	accepted := make(chan struct{}, 1)
	out := make(chan []chunkRecord, 1)

	rx := startReceiver(func(c net.Conn) {
		accepted <- struct{}{}
		conn := network.NewTCPConnection(c, &network.TCPConfig{})
		var got []chunkRecord
		for {
			header, data, err := network.ParseTransferPacket(conn)
			if err != nil {
				out <- got
				return
			}
			got = append(got, chunkRecord{index: header.ChunkIndex, data: data})
		}
	})

	return rx, out, accepted
}
