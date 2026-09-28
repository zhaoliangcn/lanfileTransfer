package discovery

import (
	"LanFileTransfer-Go/internal/network"
	"LanFileTransfer-Go/pkg/utils"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const authTokenFileName = "auth_token"

// getOrCreateAuthToken loads the shared secret used to authorise privileged
// operations (remote shutdown/restart). Stored next to the device id so both
// sides of a pairing share the same value.
func getOrCreateAuthToken() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}

	dir := filepath.Join(configDir, "LanFileTransfer")
	path := filepath.Join(dir, authTokenFileName)

	if data, err := os.ReadFile(path); err == nil {
		if token := strings.TrimSpace(string(data)); len(token) >= 32 {
			return token
		}
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		utils.SugaredLog.Errorw("failed to generate auth token", "error", err)
		return ""
	}
	token := hex.EncodeToString(raw)

	if err := os.MkdirAll(dir, 0700); err == nil {
		if err := os.WriteFile(path, []byte(token), 0600); err != nil {
			utils.SugaredLog.Errorw("failed to persist auth token", "error", err)
		}
	}

	return token
}

type Peer struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	IP       string `json:"ip"`
	Port     int    `json:"port"`
	Online   bool   `json:"online"`
	LastSeen string `json:"lastSeen"`
	// Manual marks a peer the user typed in by hand. Manual peers are exempt from
	// the discovery timeout sweep, because they will never send a broadcast.
	Manual bool `json:"manual,omitempty"`
}

type DiscoveryManager struct {
	mu                sync.RWMutex
	peers             map[string]*Peer
	udpServer         *network.UDPServer
	deviceID          string
	deviceName        string
	authToken         string
	listenPort        int
	serviceType       string
	discoveryInterval time.Duration
	ctx               context.Context
	cancel            context.CancelFunc
	stopOnce          sync.Once
	started           bool
	onPeerFound       func(*Peer)
	onPeerLost        func(string)
	onTransferReq     func(*network.ControlMessage)
	onTransferCtrl    func(*network.ControlMessage)
	onChatMessage     func(*network.ControlMessage)
	onSystemCommand   func(*network.ControlMessage)
}

func NewDiscoveryManager(deviceID, deviceName string, listenPort int, discoveryInterval int) *DiscoveryManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &DiscoveryManager{
		peers:             make(map[string]*Peer),
		deviceID:          deviceID,
		deviceName:        deviceName,
		authToken:         getOrCreateAuthToken(),
		listenPort:        listenPort,
		serviceType:       "_lanfiletransfer._tcp",
		discoveryInterval: time.Duration(discoveryInterval) * time.Second,
		ctx:               ctx,
		cancel:            cancel,
	}
}

func (dm *DiscoveryManager) Start() error {
	dm.mu.Lock()
	if dm.started {
		dm.mu.Unlock()
		return fmt.Errorf("discovery manager already started")
	}
	dm.started = true
	dm.mu.Unlock()

	udpCfg := &network.UDPConfig{
		Addr:        fmt.Sprintf(":%d", dm.listenPort),
		ReadTimeout: 30 * time.Second,
	}

	dm.udpServer = network.NewUDPServer(udpCfg)
	dm.udpServer.OnMessage(dm.handleDiscoveryMessage)

	if err := dm.udpServer.Start(); err != nil {
		return err
	}

	go dm.broadcastLoop()
	go dm.cleanupLoop()

	return nil
}

func (dm *DiscoveryManager) Stop() {
	dm.stopOnce.Do(func() {
		dm.mu.Lock()
		defer dm.mu.Unlock()

		if !dm.started {
			return
		}

		dm.cancel()
		if dm.udpServer != nil {
			dm.udpServer.Stop()
		}
		dm.started = false
	})
}

func (dm *DiscoveryManager) broadcastLoop() {
	ticker := time.NewTicker(dm.discoveryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-dm.ctx.Done():
			return
		case <-ticker.C:
			dm.broadcastPresence()
		}
	}
}

func (dm *DiscoveryManager) broadcastPresence() {
	msg := network.ControlMessage{
		Type:       "discovery",
		DeviceID:   dm.deviceID,
		DeviceName: dm.deviceName,
		Port:       dm.listenPort,
		Timestamp:  time.Now().Unix(),
	}

	data, err := network.MarshalControlMessage(&msg)
	if err != nil {
		return
	}

	if dm.udpServer != nil {
		dm.udpServer.Broadcast(data, dm.listenPort)
	}
}

func (dm *DiscoveryManager) RefreshPeers() []*Peer {
	dm.broadcastPresence()

	time.Sleep(500 * time.Millisecond)

	return dm.GetPeers()
}

func (dm *DiscoveryManager) handleDiscoveryMessage(data []byte, addr *net.UDPAddr) {
	msg, err := network.UnmarshalControlMessage(data)
	if err != nil {
		return
	}

	switch msg.Type {
	case network.ControlTypeDiscovery:
		dm.handlePeerDiscovery(msg, addr)
	case network.ControlTypeTransferRequest:
		if dm.onTransferReq != nil {
			dm.onTransferReq(msg)
		}
	case network.ControlTypeTransferControl:
		if dm.onTransferCtrl != nil {
			dm.onTransferCtrl(msg)
		}
	case network.ControlTypeChatMessage:
		if dm.onChatMessage != nil && msg.TargetID != "" && msg.TargetID == dm.deviceID {
			dm.onChatMessage(msg)
		}
	case network.ControlTypeSystemCommand:
		if dm.onSystemCommand != nil && msg.TargetID != "" && msg.TargetID == dm.deviceID {
			dm.onSystemCommand(msg)
		}
	}
}

func (dm *DiscoveryManager) handlePeerDiscovery(msg *network.ControlMessage, addr *net.UDPAddr) {
	if msg.DeviceID == dm.deviceID {
		return
	}

	dm.mu.Lock()
	existing, exists := dm.peers[msg.DeviceID]
	now := time.Now()

	if exists {
		wasOnline := existing.Online
		existing.IP = addr.IP.String()
		existing.Port = msg.Port
		existing.Name = msg.DeviceName
		existing.Online = true
		existing.LastSeen = now.Format(time.RFC3339)
		dm.mu.Unlock()

		if !wasOnline && dm.onPeerFound != nil {
			dm.onPeerFound(existing)
		}
	} else {
		peer := &Peer{
			ID:       msg.DeviceID,
			Name:     msg.DeviceName,
			IP:       addr.IP.String(),
			Port:     msg.Port,
			Online:   true,
			LastSeen: now.Format(time.RFC3339),
		}
		dm.peers[msg.DeviceID] = peer
		dm.mu.Unlock()

		if dm.onPeerFound != nil {
			dm.onPeerFound(peer)
		}
		return
	}
}

func (dm *DiscoveryManager) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-dm.ctx.Done():
			return
		case <-ticker.C:
			dm.cleanupOfflinePeers()
		}
	}
}

func (dm *DiscoveryManager) cleanupOfflinePeers() {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	now := time.Now()
	for id, peer := range dm.peers {
		// Manually added peers never broadcast, so the liveness sweep would mark
		// them offline within 30 seconds.
		if peer.Manual {
			continue
		}

		lastSeen, err := time.Parse(time.RFC3339, peer.LastSeen)
		if err != nil {
			continue
		}
		elapsed := now.Sub(lastSeen)
		if elapsed > 30*time.Second {
			if peer.Online {
				peer.Online = false
				if dm.onPeerLost != nil {
					go dm.onPeerLost(id)
				}
			}
			if elapsed > 5*time.Minute {
				delete(dm.peers, id)
			}
		}
	}
}

func (dm *DiscoveryManager) GetPeers() []*Peer {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	peers := make([]*Peer, 0, len(dm.peers))
	for _, p := range dm.peers {
		peerCopy := *p
		peers = append(peers, &peerCopy)
	}
	return peers
}

// AddManualPeer registers a peer the user entered by hand.
//
// Discovery relies on UDP broadcast, which a router never forwards, so two
// machines on different subnets (or across a VPN/tunnel) can never find each
// other automatically. This entry point lets the user supply the address, and
// transfers then work over plain TCP as long as the port is routable.
//
// The peer is exempted from the offline sweep, and re-adding the same address
// updates it in place instead of creating a duplicate.
func (dm *DiscoveryManager) AddManualPeer(name, address string, port int) (*Peer, error) {
	host, err := normalizeHost(address)
	if err != nil {
		return nil, err
	}

	if port <= 0 || port > 65535 {
		port = dm.listenPort
	}

	id := "manual-" + host
	if name == "" {
		name = host
	}

	dm.mu.Lock()
	now := time.Now().Format(time.RFC3339)
	peer, exists := dm.peers[id]
	if exists {
		peer.Name = name
		peer.IP = host
		peer.Port = port
		peer.Online = true
		peer.Manual = true
		peer.LastSeen = now
	} else {
		peer = &Peer{
			ID:       id,
			Name:     name,
			IP:       host,
			Port:     port,
			Online:   true,
			Manual:   true,
			LastSeen: now,
		}
		dm.peers[id] = peer
	}
	peerCopy := *peer
	callback := dm.onPeerFound
	dm.mu.Unlock()

	if callback != nil {
		go callback(&peerCopy)
	}

	utils.SugaredLog.Infow("manual peer registered", "ip", host, "port", port, "name", name)
	return &peerCopy, nil
}

// RemoveManualPeer drops a manually added peer. Discovered peers are left
// alone since they will simply be rediscovered.
func (dm *DiscoveryManager) RemoveManualPeer(id string) bool {
	dm.mu.Lock()
	peer, exists := dm.peers[id]
	if !exists || !peer.Manual {
		dm.mu.Unlock()
		return false
	}
	delete(dm.peers, id)
	dm.mu.Unlock()
	return true
}

// ProbePeer reports whether a peer currently accepts a TCP connection on its
// transfer port. Used by the UI to validate a manually entered address.
func (dm *DiscoveryManager) ProbePeer(address string, port int) error {
	host, err := normalizeHost(address)
	if err != nil {
		return err
	}
	if port <= 0 || port > 65535 {
		port = dm.listenPort
	}

	conn, err := net.DialTimeout("tcp",
		net.JoinHostPort(host, strconv.Itoa(port)), 3*time.Second)
	if err != nil {
		return fmt.Errorf("cannot reach %s:%d: %w", host, port, err)
	}
	return conn.Close()
}

// normalizeHost accepts a bare address or a "host:port" string and rejects
// anything that is not a valid IP or resolvable hostname.
func normalizeHost(address string) (string, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return "", fmt.Errorf("address is empty")
	}

	// Accept "1.2.3.4:9876" or "[::1]:9876" and keep only the host part.
	if strings.HasPrefix(address, "[") {
		if end := strings.Index(address, "]"); end > 0 {
			address = address[1:end]
		}
	} else if host, _, err := net.SplitHostPort(address); err == nil && host != "" {
		address = host
	}

	if ip := net.ParseIP(address); ip != nil {
		return ip.String(), nil
	}

	if host, err := net.LookupHost(address); err == nil && len(host) > 0 {
		return host[0], nil
	}

	return "", fmt.Errorf("invalid address: %q", address)
}

func (dm *DiscoveryManager) GetPeer(id string) *Peer {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	peer, exists := dm.peers[id]
	if !exists {
		return nil
	}
	peerCopy := *peer
	return &peerCopy
}

func (dm *DiscoveryManager) OnPeerFound(callback func(*Peer)) {
	dm.onPeerFound = callback
}

func (dm *DiscoveryManager) OnPeerLost(callback func(string)) {
	dm.onPeerLost = callback
}

func (dm *DiscoveryManager) OnTransferRequest(callback func(*network.ControlMessage)) {
	dm.onTransferReq = callback
}

func (dm *DiscoveryManager) OnTransferControl(callback func(*network.ControlMessage)) {
	dm.onTransferCtrl = callback
}

func (dm *DiscoveryManager) OnChatMessage(callback func(*network.ControlMessage)) {
	dm.onChatMessage = callback
}

func (dm *DiscoveryManager) OnSystemCommand(callback func(*network.ControlMessage)) {
	dm.onSystemCommand = callback
}

// VerifyAuthToken reports whether token matches this device's shared secret.
// Uses a constant-time comparison so a wrong token cannot be recovered by
// measuring response latency.
func (dm *DiscoveryManager) VerifyAuthToken(token string) bool {
	expected := dm.authToken
	if expected == "" || token == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(token)) == 1
}

// AuthToken returns this device's shared secret (for pairing with a peer).
func (dm *DiscoveryManager) AuthToken() string {
	return dm.authToken
}

func (dm *DiscoveryManager) SendChatMessage(targetPeerID, content string) error {
	dm.mu.RLock()
	peer, exists := dm.peers[targetPeerID]
	dm.mu.RUnlock()

	if !exists || !peer.Online {
		return fmt.Errorf("peer %s is not online", targetPeerID)
	}

	msg := network.ControlMessage{
		Type:       network.ControlTypeChatMessage,
		DeviceID:   dm.deviceID,
		DeviceName: dm.deviceName,
		TargetID:   targetPeerID,
		Message:    content,
		Timestamp:  time.Now().Unix(),
	}

	data, err := network.MarshalControlMessage(&msg)
	if err != nil {
		return err
	}

	addr := &net.UDPAddr{
		IP:   net.ParseIP(peer.IP),
		Port: peer.Port,
	}

	return dm.udpServer.SendTo(data, addr)
}

func (dm *DiscoveryManager) SendSystemCommand(peerID, command string) error {
	dm.mu.RLock()
	peer, exists := dm.peers[peerID]
	dm.mu.RUnlock()

	if !exists || !peer.Online {
		utils.SugaredLog.Warnw("SendSystemCommand: peer not online",
			"peerID", peerID,
			"exists", exists,
		)
		return fmt.Errorf("peer %s is not online", peerID)
	}

	msg := network.ControlMessage{
		Type:       network.ControlTypeSystemCommand,
		DeviceID:   dm.deviceID,
		DeviceName: dm.deviceName,
		TargetID:   peerID,
		Command:    command,
		AuthToken:  dm.authToken,
		Timestamp:  time.Now().Unix(),
	}

	data, err := network.MarshalControlMessage(&msg)
	if err != nil {
		return err
	}

	addr := &net.UDPAddr{
		IP:   net.ParseIP(peer.IP),
		Port: peer.Port,
	}

	utils.SugaredLog.Infow("sending system command via UDP",
		"targetIP", peer.IP,
		"targetPort", peer.Port,
		"targetID", peerID,
		"command", command,
		"dataLen", len(data),
	)

	return dm.udpServer.SendTo(data, addr)
}

func (dm *DiscoveryManager) SendTransferRequest(peerAddr string, transferID, fileName string, fileSize int64) error {
	msg := network.ControlMessage{
		Type:       network.ControlTypeTransferRequest,
		TransferID: transferID,
		FileName:   fileName,
		FileSize:   fileSize,
		DeviceID:   dm.deviceID,
		DeviceName: dm.deviceName,
		Timestamp:  time.Now().Unix(),
	}

	data, err := network.MarshalControlMessage(&msg)
	if err != nil {
		return err
	}

	return network.SendUDPBroadcast(data, dm.listenPort)
}

func (dm *DiscoveryManager) SendTransferControl(transferID, command string) error {
	msg := network.ControlMessage{
		Type:       network.ControlTypeTransferControl,
		TransferID: transferID,
		Command:    command,
		DeviceID:   dm.deviceID,
		DeviceName: dm.deviceName,
		Timestamp:  time.Now().Unix(),
	}

	data, err := network.MarshalControlMessage(&msg)
	if err != nil {
		return err
	}

	return network.SendUDPBroadcast(data, dm.listenPort)
}

func (dm *DiscoveryManager) GetLocalPeer() *Peer {
	return &Peer{
		ID:       dm.deviceID,
		Name:     dm.deviceName,
		IP:       network.GetLocalIP(),
		Port:     dm.listenPort,
		Online:   true,
		LastSeen: time.Now().Format(time.RFC3339),
	}
}
