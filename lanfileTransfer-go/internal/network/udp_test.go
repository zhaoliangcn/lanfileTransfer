package network

import (
	"net"
	"sync"
	"testing"
	"time"
)

func TestUDPServerStartStop(t *testing.T) {
	cfg := &UDPConfig{
		Addr: "127.0.0.1:0",
	}
	server := NewUDPServer(cfg)

	err := server.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	localAddr := server.LocalAddr()
	if localAddr == nil {
		t.Errorf("LocalAddr should not be nil after start")
	}

	server.Stop()
}

func TestUDPServerSendTo(t *testing.T) {
	cfg := &UDPConfig{
		Addr: "127.0.0.1:0",
	}
	server := NewUDPServer(cfg)

	err := server.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer server.Stop()

	received := make(chan []byte, 1)
	listenerCfg := &UDPConfig{
		Addr: "127.0.0.1:0",
	}
	listener := NewUDPServer(listenerCfg)
	listener.OnMessage(func(data []byte, addr *net.UDPAddr) {
		received <- data
	})

	err = listener.Start()
	if err != nil {
		t.Fatalf("listener Start failed: %v", err)
	}
	defer listener.Stop()

	time.Sleep(50 * time.Millisecond)
	data := []byte("udp test message")
	err = server.SendTo(data, listener.LocalAddr())
	if err != nil {
		t.Fatalf("SendTo failed: %v", err)
	}

	select {
	case msg := <-received:
		if string(msg) != string(data) {
			t.Errorf("message mismatch: got '%s', want '%s'", string(msg), string(data))
		}
	case <-time.After(2 * time.Second):
		t.Errorf("timeout waiting for message")
	}
}

func TestUDPServerSendBeforeStart(t *testing.T) {
	cfg := &UDPConfig{Addr: "127.0.0.1:0"}
	server := NewUDPServer(cfg)

	err := server.SendTo([]byte("test"), &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 9999})
	if err == nil {
		t.Errorf("expected error sending before start")
	}
}

func TestUDPServerBroadcast(t *testing.T) {
	cfg := &UDPConfig{
		Addr: "127.0.0.1:0",
	}
	server := NewUDPServer(cfg)

	err := server.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer server.Stop()

	time.Sleep(50 * time.Millisecond)
	err = server.Broadcast([]byte("broadcast test"), 9999)
	if err != nil {
		t.Logf("Broadcast failed (expected in some environments): %v", err)
	}
}

func TestUDPServerBroadcastBeforeStart(t *testing.T) {
	cfg := &UDPConfig{Addr: "127.0.0.1:0"}
	server := NewUDPServer(cfg)

	err := server.Broadcast([]byte("test"), 9999)
	if err == nil {
		t.Errorf("expected error broadcasting before start")
	}
}

func TestUDPServerOnMessage(t *testing.T) {
	var mu sync.Mutex
	received := false

	cfg := &UDPConfig{
		Addr: "127.0.0.1:0",
	}
	server := NewUDPServer(cfg)
	server.OnMessage(func(data []byte, addr *net.UDPAddr) {
		mu.Lock()
		received = true
		mu.Unlock()
	})

	err := server.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer server.Stop()

	senderCfg := &UDPConfig{Addr: "127.0.0.1:0"}
	sender := NewUDPServer(senderCfg)
	sender.Start()
	defer sender.Stop()

	time.Sleep(50 * time.Millisecond)
	sender.SendTo([]byte("hello"), server.LocalAddr())

	time.Sleep(200 * time.Millisecond)
	mu.Lock()
	if !received {
		t.Errorf("OnMessage should have been called")
	}
	mu.Unlock()
}

func TestUDPServerLocalAddrBeforeStart(t *testing.T) {
	cfg := &UDPConfig{Addr: "127.0.0.1:0"}
	server := NewUDPServer(cfg)

	addr := server.LocalAddr()
	if addr != nil {
		t.Errorf("LocalAddr should be nil before start")
	}
}

func TestSendUDPBroadcast(t *testing.T) {
	err := SendUDPBroadcast([]byte("broadcast"), 9998)
	if err != nil {
		t.Logf("SendUDPBroadcast failed (expected in some environments): %v", err)
	}
}

func TestUDPReadTimeout(t *testing.T) {
	cfg := &UDPConfig{
		Addr:        "127.0.0.1:0",
		ReadTimeout: 100 * time.Millisecond,
	}
	server := NewUDPServer(cfg)

	err := server.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)
	server.Stop()
}

func TestUDPServerDoubleStop(t *testing.T) {
	cfg := &UDPConfig{Addr: "127.0.0.1:0"}
	server := NewUDPServer(cfg)

	server.Start()
	server.Stop()
	server.Stop()
}

func TestUDPServerMessageHandler(t *testing.T) {
	serverCfg := &UDPConfig{
		Addr:        "127.0.0.1:0",
		ReadTimeout: 1 * time.Second,
	}
	server := NewUDPServer(serverCfg)

	msgChan := make(chan []byte, 1)
	server.OnMessage(func(data []byte, addr *net.UDPAddr) {
		msgChan <- data
	})

	server.Start()
	defer server.Stop()

	conn, err := net.DialUDP("udp", nil, server.LocalAddr())
	if err != nil {
		t.Fatalf("DialUDP failed: %v", err)
	}
	defer conn.Close()

	testData := []byte("direct udp message")
	_, err = conn.Write(testData)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	select {
	case msg := <-msgChan:
		if string(msg) != string(testData) {
			t.Errorf("message mismatch: got '%s', want '%s'", string(msg), string(testData))
		}
	case <-time.After(2 * time.Second):
		t.Errorf("timeout waiting for UDP message")
	}
}
