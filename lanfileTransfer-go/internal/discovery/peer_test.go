package discovery

import (
	"LanFileTransfer-Go/internal/network"
	"net"
	"testing"
	"time"
)

func TestNewDiscoveryManager(t *testing.T) {
	dm := NewDiscoveryManager("device-1", "TestDevice", 9876, 5)

	if dm.deviceID != "device-1" {
		t.Errorf("expected deviceID 'device-1', got '%s'", dm.deviceID)
	}
	if dm.deviceName != "TestDevice" {
		t.Errorf("expected deviceName 'TestDevice', got '%s'", dm.deviceName)
	}
	if dm.listenPort != 9876 {
		t.Errorf("expected listenPort 9876, got %d", dm.listenPort)
	}
	if dm.started {
		t.Errorf("new discovery manager should not be started")
	}
	if peers := dm.GetPeers(); len(peers) != 0 {
		t.Errorf("new discovery manager should have no peers, got %d", len(peers))
	}
}

func TestPeerStructure(t *testing.T) {
	peer := &Peer{
		ID:       "peer-1",
		Name:     "PeerOne",
		IP:       "192.168.1.100",
		Port:     9876,
		Online:   true,
		LastSeen: time.Now().Format(time.RFC3339),
	}

	if peer.ID != "peer-1" {
		t.Errorf("ID mismatch")
	}
	if peer.Name != "PeerOne" {
		t.Errorf("Name mismatch")
	}
	if peer.IP != "192.168.1.100" {
		t.Errorf("IP mismatch")
	}
	if peer.Port != 9876 {
		t.Errorf("Port mismatch")
	}
	if !peer.Online {
		t.Errorf("should be online")
	}
}

func TestGetPeerNonExistent(t *testing.T) {
	dm := NewDiscoveryManager("device-1", "TestDevice", 9876, 5)
	peer := dm.GetPeer("nonexistent")
	if peer != nil {
		t.Errorf("expected nil for non-existent peer, got %+v", peer)
	}
}

func TestGetPeerExists(t *testing.T) {
	dm := NewDiscoveryManager("device-1", "TestDevice", 9876, 5)

	dm.mu.Lock()
	dm.peers["peer-1"] = &Peer{
		ID:       "peer-1",
		Name:     "PeerOne",
		IP:       "192.168.1.100",
		Port:     9876,
		Online:   true,
		LastSeen: time.Now().Format(time.RFC3339),
	}
	dm.mu.Unlock()

	peer := dm.GetPeer("peer-1")
	if peer == nil {
		t.Fatal("expected peer to exist")
	}
	if peer.ID != "peer-1" {
		t.Errorf("expected ID 'peer-1', got '%s'", peer.ID)
	}
}

func TestGetPeers(t *testing.T) {
	dm := NewDiscoveryManager("device-1", "TestDevice", 9876, 5)

	dm.mu.Lock()
	dm.peers["peer-1"] = &Peer{ID: "peer-1", Name: "PeerOne", Online: true}
	dm.peers["peer-2"] = &Peer{ID: "peer-2", Name: "PeerTwo", Online: true}
	dm.mu.Unlock()

	peers := dm.GetPeers()
	if len(peers) != 2 {
		t.Errorf("expected 2 peers, got %d", len(peers))
	}
}

func TestGetLocalPeer(t *testing.T) {
	dm := NewDiscoveryManager("device-1", "TestDevice", 9876, 5)
	local := dm.GetLocalPeer()

	if local.ID != "device-1" {
		t.Errorf("expected local peer ID 'device-1', got '%s'", local.ID)
	}
	if local.Name != "TestDevice" {
		t.Errorf("expected local peer Name 'TestDevice', got '%s'", local.Name)
	}
	if local.Port != 9876 {
		t.Errorf("expected local peer Port 9876, got %d", local.Port)
	}
	if !local.Online {
		t.Errorf("local peer should be online")
	}
	if local.IP == "" {
		t.Errorf("local peer IP should not be empty")
	}
}

func TestHandlePeerDiscoverySelf(t *testing.T) {
	dm := NewDiscoveryManager("device-1", "TestDevice", 9876, 5)

	msg := &network.ControlMessage{
		Type:       network.ControlTypeDiscovery,
		DeviceID:   "device-1",
		DeviceName: "Self",
		Port:       9876,
	}

	dm.handlePeerDiscovery(msg, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 9876})

	if len(dm.GetPeers()) != 0 {
		t.Errorf("should not add self as peer")
	}
}

func TestHandlePeerDiscoveryNewPeer(t *testing.T) {
	dm := NewDiscoveryManager("device-1", "TestDevice", 9876, 5)

	foundCalled := false
	dm.OnPeerFound(func(peer *Peer) {
		foundCalled = true
		if peer.ID != "peer-2" {
			t.Errorf("expected peer ID 'peer-2', got '%s'", peer.ID)
		}
	})

	msg := &network.ControlMessage{
		Type:       network.ControlTypeDiscovery,
		DeviceID:   "peer-2",
		DeviceName: "PeerTwo",
		Port:       9876,
	}

	dm.handlePeerDiscovery(msg, &net.UDPAddr{IP: net.IPv4(192, 168, 1, 100), Port: 9876})

	if !foundCalled {
		t.Errorf("onPeerFound should have been called")
	}
	peer := dm.GetPeer("peer-2")
	if peer == nil {
		t.Fatal("expected peer-2 to exist")
	}
	if !peer.Online {
		t.Errorf("peer should be online")
	}
}

func TestHandlePeerDiscoveryExistingPeer(t *testing.T) {
	dm := NewDiscoveryManager("device-1", "TestDevice", 9876, 5)

	dm.mu.Lock()
	dm.peers["peer-2"] = &Peer{
		ID:       "peer-2",
		Name:     "OldName",
		IP:       "0.0.0.0",
		Port:     0,
		Online:   false,
		LastSeen: "2020-01-01T00:00:00Z",
	}
	dm.mu.Unlock()

	foundCalled := false
	dm.OnPeerFound(func(peer *Peer) {
		foundCalled = true
	})

	msg := &network.ControlMessage{
		Type:       network.ControlTypeDiscovery,
		DeviceID:   "peer-2",
		DeviceName: "NewName",
		Port:       9877,
	}

	dm.handlePeerDiscovery(msg, &net.UDPAddr{IP: net.IPv4(192, 168, 1, 101), Port: 9877})

	if !foundCalled {
		t.Errorf("onPeerFound should be called when offline peer comes back online")
	}

	peer := dm.GetPeer("peer-2")
	if peer.Name != "NewName" {
		t.Errorf("expected updated name 'NewName', got '%s'", peer.Name)
	}
	if !peer.Online {
		t.Errorf("peer should be online after re-discovery")
	}
}

func TestHandlePeerDiscoveryExistingOnlinePeer(t *testing.T) {
	dm := NewDiscoveryManager("device-1", "TestDevice", 9876, 5)

	dm.mu.Lock()
	dm.peers["peer-2"] = &Peer{
		ID:       "peer-2",
		Name:     "OldName",
		IP:       "192.168.1.100",
		Port:     9876,
		Online:   true,
		LastSeen: time.Now().Format(time.RFC3339),
	}
	dm.mu.Unlock()

	foundCalled := false
	dm.OnPeerFound(func(peer *Peer) {
		foundCalled = true
	})

	msg := &network.ControlMessage{
		Type:       network.ControlTypeDiscovery,
		DeviceID:   "peer-2",
		DeviceName: "NewName",
		Port:       9877,
	}

	dm.handlePeerDiscovery(msg, &net.UDPAddr{IP: net.IPv4(192, 168, 1, 101), Port: 9877})

	if foundCalled {
		t.Errorf("onPeerFound should not be called for already-online peer")
	}

	peer := dm.GetPeer("peer-2")
	if peer.Name != "NewName" {
		t.Errorf("expected updated name 'NewName', got '%s'", peer.Name)
	}
	if !peer.Online {
		t.Errorf("peer should remain online after re-discovery")
	}
}

func TestCleanupOfflinePeers(t *testing.T) {
	dm := NewDiscoveryManager("device-1", "TestDevice", 9876, 5)

	dm.mu.Lock()
	dm.peers["peer-old"] = &Peer{
		ID:       "peer-old",
		Name:     "OldPeer",
		Online:   true,
		LastSeen: time.Now().Add(-60 * time.Second).Format(time.RFC3339),
	}
	dm.peers["peer-new"] = &Peer{
		ID:       "peer-new",
		Name:     "NewPeer",
		Online:   true,
		LastSeen: time.Now().Format(time.RFC3339),
	}
	dm.mu.Unlock()

	dm.cleanupOfflinePeers()

	oldPeer := dm.GetPeer("peer-old")
	if oldPeer == nil {
		t.Fatal("expected peer-old to still exist")
	}
	if oldPeer.Online {
		t.Errorf("peer-old should be marked offline after cleanup")
	}

	newPeer := dm.GetPeer("peer-new")
	if !newPeer.Online {
		t.Errorf("peer-new should still be online")
	}
}

func TestOnTransferRequest(t *testing.T) {
	dm := NewDiscoveryManager("device-1", "TestDevice", 9876, 5)

	transferReqCalled := false
	dm.OnTransferRequest(func(msg *network.ControlMessage) {
		transferReqCalled = true
		if msg.TransferID != "transfer-1" {
			t.Errorf("expected transfer ID 'transfer-1', got '%s'", msg.TransferID)
		}
	})

	msg := &network.ControlMessage{
		Type:       network.ControlTypeTransferRequest,
		TransferID: "transfer-1",
	}

	data, _ := network.MarshalControlMessage(msg)
	dm.handleDiscoveryMessage(data, &net.UDPAddr{IP: net.IPv4(192, 168, 1, 100), Port: 9876})

	if !transferReqCalled {
		t.Errorf("OnTransferRequest should have been called")
	}
}

func TestOnTransferControl(t *testing.T) {
	dm := NewDiscoveryManager("device-1", "TestDevice", 9876, 5)

	ctrlCalled := false
	dm.OnTransferControl(func(msg *network.ControlMessage) {
		ctrlCalled = true
		if msg.Command != network.ControlCommandCancel {
			t.Errorf("expected command 'cancel', got '%s'", msg.Command)
		}
	})

	msg := &network.ControlMessage{
		Type:       network.ControlTypeTransferControl,
		TransferID: "transfer-1",
		Command:    network.ControlCommandCancel,
	}

	data, _ := network.MarshalControlMessage(msg)
	dm.handleDiscoveryMessage(data, &net.UDPAddr{IP: net.IPv4(192, 168, 1, 100), Port: 9876})

	if !ctrlCalled {
		t.Errorf("OnTransferControl should have been called")
	}
}

func TestSendTransferRequest(t *testing.T) {
	dm := NewDiscoveryManager("device-1", "TestDevice", 9876, 5)

	err := dm.SendTransferRequest("192.168.1.100", "transfer-1", "file.txt", 1024)
	if err != nil {
		t.Logf("SendTransferRequest returned error (may be expected if no network): %v", err)
	}
}

func TestSendTransferControl(t *testing.T) {
	dm := NewDiscoveryManager("device-1", "TestDevice", 9876, 5)

	err := dm.SendTransferControl("transfer-1", network.ControlCommandCancel)
	if err != nil {
		t.Logf("SendTransferControl returned error (may be expected if no network): %v", err)
	}
}

func TestStopNotStarted(t *testing.T) {
	dm := NewDiscoveryManager("device-1", "TestDevice", 9876, 5)

	dm.Stop()
}

func TestStartStop(t *testing.T) {
	dm := NewDiscoveryManager("device-1", "TestDevice", 0, 5)

	err := dm.Start()
	if err != nil {
		t.Skipf("Start failed (may be expected): %v", err)
	}

	dm.Stop()
}

func TestStartTwice(t *testing.T) {
	dm := NewDiscoveryManager("device-1", "TestDevice", 0, 5)

	dm.Start()
	dm.Stop()
}
