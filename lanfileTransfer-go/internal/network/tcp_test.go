package network

import (
	"net"
	"sync"
	"testing"
	"time"
)

func TestTCPConnectionSend(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go func() {
		buf := make([]byte, 256)
		server.Read(buf)
	}()

	cfg := &TCPConfig{WriteTimeout: 5 * time.Second}
	conn := NewTCPConnection(client, cfg)

	data := []byte("hello tcp connection")
	err := conn.Send(data)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
}

func TestTCPConnectionRead(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	cfg := &TCPConfig{ReadTimeout: 5 * time.Second}
	conn := NewTCPConnection(client, cfg)

	go func() {
		server.Write([]byte("test read data"))
	}()

	buf := make([]byte, 14)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if n != 14 {
		t.Errorf("read %d bytes, expected 14", n)
	}
	if string(buf[:n]) != "test read data" {
		t.Errorf("read data mismatch: got '%s'", string(buf[:n]))
	}
}

func TestTCPConnectionClose(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	cfg := &TCPConfig{}
	conn := NewTCPConnection(client, cfg)

	err := conn.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	err = conn.Send([]byte("data after close"))
	if err == nil {
		t.Errorf("expected error sending on closed connection")
	}
}

func TestTCPConnectionDoubleClose(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	cfg := &TCPConfig{}
	conn := NewTCPConnection(client, cfg)

	if err := conn.Close(); err != nil {
		t.Fatalf("first Close failed: %v", err)
	}

	if err := conn.Close(); err != nil {
		t.Errorf("second Close should not error, got: %v", err)
	}
}

func TestTCPConnectionRemoteAddr(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer listener.Close()

	addr := listener.Addr().String()

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			return
		}
		defer conn.Close()
	}()

	serverConn, err := listener.Accept()
	if err != nil {
		t.Fatalf("accept failed: %v", err)
	}
	defer serverConn.Close()

	cfg := &TCPConfig{}
	tcpConn := NewTCPConnection(serverConn, cfg)

	remoteAddr := tcpConn.RemoteAddr()
	if remoteAddr == nil {
		t.Errorf("RemoteAddr should not be nil")
	}

	wg.Wait()
}

func TestTCPServerStartStop(t *testing.T) {
	cfg := &TCPConfig{
		Addr: "127.0.0.1:0",
	}
	server := NewTCPServer(cfg)

	err := server.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	connectCalled := false
	server.OnConnect(func(conn *TCPConnection) {
		connectCalled = true
	})

	addr := server.listener.Addr().String()
	clientConn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)
	if !connectCalled {
		t.Errorf("OnConnect should have been called")
	}

	conn, exists := server.GetConnection(clientConn.LocalAddr().String())
	if !exists {
		t.Errorf("connection should exist in server")
	} else {
		conn.Close()
	}

	clientConn.Close()
	time.Sleep(100 * time.Millisecond)

	server.Stop()
}

func TestTCPServerGetConnectionNotFound(t *testing.T) {
	cfg := &TCPConfig{Addr: "127.0.0.1:0"}
	server := NewTCPServer(cfg)
	server.Start()

	_, exists := server.GetConnection("nonexistent:1234")
	if exists {
		t.Errorf("should not find non-existent connection")
	}

	server.Stop()
}

func TestDialTCP(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	defer listener.Close()

	go func() {
		conn, _ := listener.Accept()
		if conn != nil {
			conn.Close()
		}
	}()

	cfg := &TCPConfig{ReadTimeout: 3 * time.Second}
	conn, err := DialTCP(listener.Addr().String(), cfg)
	if err != nil {
		t.Fatalf("DialTCP failed: %v", err)
	}
	defer conn.Close()

	if conn.RemoteAddr() == nil {
		t.Errorf("RemoteAddr should not be nil")
	}
}

func TestDialTCPTimeout(t *testing.T) {
	cfg := &TCPConfig{ReadTimeout: 3 * time.Second}
	_, err := DialTCP("127.0.0.1:19999", cfg)
	if err == nil {
		t.Errorf("expected error dialing unreachable address")
	}
}

func TestTCPConnectionReadTimeout(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	cfg := &TCPConfig{ReadTimeout: 100 * time.Millisecond}
	conn := NewTCPConnection(client, cfg)

	buf := make([]byte, 10)
	_, err := conn.Read(buf)
	if err == nil {
		t.Errorf("expected timeout error")
	}
}

func TestTCPConnectionConcurrentSend(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	cfg := &TCPConfig{WriteTimeout: 5 * time.Second}
	conn := NewTCPConnection(client, cfg)

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			data := []byte{byte(n)}
			err := conn.Send(data)
			if err != nil {
				t.Logf("concurrent send %d failed: %v", n, err)
			}
		}(i)
	}

	go func() {
		received := 0
		for received < 5 {
			buf := make([]byte, 1)
			server.Read(buf)
			received++
		}
	}()

	wg.Wait()
}
