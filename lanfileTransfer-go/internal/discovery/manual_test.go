package discovery

import (
	"testing"
	"time"
)

// --- manual peer support ---

func TestAddManualPeer(t *testing.T) {
	dm := NewDiscoveryManager("device-1", "TestDevice", 9876, 5)

	peer, err := dm.AddManualPeer("Office PC", "192.168.0.100", 0)
	if err != nil {
		t.Fatalf("AddManualPeer failed: %v", err)
	}

	if peer.Name != "Office PC" {
		t.Errorf("Name = %q, want %q", peer.Name, "Office PC")
	}
	if peer.IP != "192.168.0.100" {
		t.Errorf("IP = %q, want %q", peer.IP, "192.168.0.100")
	}
	if peer.Port != 9876 {
		t.Errorf("Port = %d, want the manager default 9876", peer.Port)
	}
	if !peer.Manual {
		t.Error("Manual = false, want true")
	}
	if !peer.Online {
		t.Error("Online = false, want true")
	}

	found := dm.GetPeer(peer.ID)
	if found == nil {
		t.Fatalf("GetPeer(%q) = nil", peer.ID)
	}
	if !found.Manual {
		t.Error("stored peer lost the Manual flag")
	}
}

func TestAddManualPeerIsIdempotent(t *testing.T) {
	dm := NewDiscoveryManager("device-1", "TestDevice", 9876, 5)

	first, err := dm.AddManualPeer("A", "192.168.0.100", 0)
	if err != nil {
		t.Fatalf("AddManualPeer: %v", err)
	}
	second, err := dm.AddManualPeer("B", "192.168.0.100", 9999)
	if err != nil {
		t.Fatalf("AddManualPeer second: %v", err)
	}

	if first.ID != second.ID {
		t.Errorf("ids differ: %q vs %q", first.ID, second.ID)
	}
	if got := len(dm.GetPeers()); got != 1 {
		t.Errorf("peer count = %d, want 1 (no duplicate)", got)
	}
	if second.Name != "B" {
		t.Errorf("Name = %q, want it updated to %q", second.Name, "B")
	}
	if second.Port != 9999 {
		t.Errorf("Port = %d, want it updated to 9999", second.Port)
	}
}

func TestAddManualPeerAcceptsHostPort(t *testing.T) {
	dm := NewDiscoveryManager("device-1", "TestDevice", 9876, 5)

	peer, err := dm.AddManualPeer("", "192.168.0.100:12345", 0)
	if err != nil {
		t.Fatalf("AddManualPeer: %v", err)
	}
	if peer.IP != "192.168.0.100" {
		t.Errorf("IP = %q, want the host part only", peer.IP)
	}
	if peer.Name != "192.168.0.100" {
		t.Errorf("Name = %q, want a default derived from the address", peer.Name)
	}
}

func TestAddManualPeerRejectsGarbage(t *testing.T) {
	dm := NewDiscoveryManager("device-1", "TestDevice", 9876, 5)

	for _, addr := range []string{"", "   ", "not a host!!", "999.999.999.999"} {
		if _, err := dm.AddManualPeer("x", addr, 0); err == nil {
			t.Errorf("AddManualPeer(%q) succeeded, want an error", addr)
		}
	}
}

func TestRemoveManualPeer(t *testing.T) {
	dm := NewDiscoveryManager("device-1", "TestDevice", 9876, 5)

	peer, err := dm.AddManualPeer("Office", "192.168.0.100", 0)
	if err != nil {
		t.Fatalf("AddManualPeer: %v", err)
	}

	if !dm.RemoveManualPeer(peer.ID) {
		t.Error("RemoveManualPeer returned false, want true")
	}
	if dm.GetPeer(peer.ID) != nil {
		t.Error("peer still present after removal")
	}
	if dm.RemoveManualPeer(peer.ID) {
		t.Error("second removal returned true, want false")
	}
}

func TestRemoveManualPeerIgnoresDiscovered(t *testing.T) {
	dm := NewDiscoveryManager("device-1", "TestDevice", 9876, 5)

	dm.mu.Lock()
	dm.peers["discovered-1"] = &Peer{ID: "discovered-1", Name: "n", IP: "192.168.1.5", Online: true}
	dm.mu.Unlock()

	if dm.RemoveManualPeer("discovered-1") {
		t.Error("removed a discovered peer, want it left alone")
	}
	if dm.GetPeer("discovered-1") == nil {
		t.Error("discovered peer was deleted")
	}
}

// A manual peer will never answer a discovery broadcast, so the liveness sweep
// used to mark it offline within 30 seconds.
func TestManualPeerSurvivesOfflineSweep(t *testing.T) {
	dm := NewDiscoveryManager("device-1", "TestDevice", 9876, 5)

	peer, err := dm.AddManualPeer("Office", "192.168.0.100", 0)
	if err != nil {
		t.Fatalf("AddManualPeer: %v", err)
	}

	dm.mu.Lock()
	dm.peers[peer.ID].LastSeen = time.Now().Add(-time.Hour).Format(time.RFC3339)
	dm.mu.Unlock()

	dm.cleanupOfflinePeers()

	after := dm.GetPeer(peer.ID)
	if after == nil {
		t.Fatal("manual peer was deleted by the sweep")
	}
	if !after.Online {
		t.Error("manual peer was marked offline by the sweep")
	}
}

func TestDiscoveredPeerStillGoesOffline(t *testing.T) {
	dm := NewDiscoveryManager("device-1", "TestDevice", 9876, 5)

	dm.mu.Lock()
	dm.peers["discovered-1"] = &Peer{
		ID: "discovered-1", Name: "n", IP: "192.168.1.5",
		Online: true, LastSeen: time.Now().Add(-time.Hour).Format(time.RFC3339),
	}
	dm.mu.Unlock()

	dm.cleanupOfflinePeers()

	// Past the 5 minute retention the entry should be dropped entirely.
	if after := dm.GetPeer("discovered-1"); after != nil {
		if after.Online {
			t.Error("discovered peer stayed online past the timeout")
		} else {
			t.Error("peer older than 5 minutes should have been evicted, not just marked offline")
		}
	}
}

func TestVerifyAuthToken(t *testing.T) {
	dm := NewDiscoveryManager("device-1", "TestDevice", 9876, 5)
	dm.authToken = "0123456789abcdef0123456789abcdef"

	if !dm.VerifyAuthToken("0123456789abcdef0123456789abcdef") {
		t.Error("correct token rejected")
	}
	for _, bad := range []string{
		"",
		"wrong",
		"0123456789abcdef0123456789abcde",
		"0123456789ABCDEF0123456789abcdef",
	} {
		if dm.VerifyAuthToken(bad) {
			t.Errorf("VerifyAuthToken(%q) = true, want false", bad)
		}
	}
}

func TestVerifyAuthTokenRejectsWhenUnset(t *testing.T) {
	dm := NewDiscoveryManager("device-1", "TestDevice", 9876, 5)
	dm.authToken = ""

	if dm.VerifyAuthToken("anything") {
		t.Error("accepted a token when none is configured")
	}
}

func TestAuthTokenIsStable(t *testing.T) {
	a := NewDiscoveryManager("device-1", "TestDevice", 9876, 5)
	b := NewDiscoveryManager("device-2", "Other", 9876, 5)

	if a.AuthToken() == "" {
		t.Skip("no writable config dir in this environment")
	}
	if a.AuthToken() != b.AuthToken() {
		t.Error("two managers generated different tokens; pairing would never match")
	}
}

func TestProbePeerUnreachable(t *testing.T) {
	dm := NewDiscoveryManager("device-1", "TestDevice", 9876, 5)

	// Port 1 on loopback is not expected to be listening.
	if err := dm.ProbePeer("127.0.0.1", 1); err == nil {
		t.Error("ProbePeer on a closed port succeeded, want an error")
	}
}

func TestProbePeerRejectsGarbage(t *testing.T) {
	dm := NewDiscoveryManager("device-1", "TestDevice", 9876, 5)

	if err := dm.ProbePeer("not a host!!", 9876); err == nil {
		t.Error("ProbePeer accepted an invalid address")
	}
}
