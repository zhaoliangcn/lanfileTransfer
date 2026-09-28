package service

import (
	"LanFileTransfer-Go/internal/discovery"
	"fmt"
	"time"
)

type PeerService struct {
	discoveryMgr *discovery.DiscoveryManager
}

func NewPeerService(discoveryMgr *discovery.DiscoveryManager) *PeerService {
	return &PeerService{
		discoveryMgr: discoveryMgr,
	}
}

func (s *PeerService) GetPeers() []*discovery.Peer {
	return s.discoveryMgr.GetPeers()
}

func (s *PeerService) RefreshPeers() []*discovery.Peer {
	return s.discoveryMgr.RefreshPeers()
}

func (s *PeerService) GetPeer(id string) *discovery.Peer {
	return s.discoveryMgr.GetPeer(id)
}

func (s *PeerService) GetLocalPeer() *discovery.Peer {
	return s.discoveryMgr.GetLocalPeer()
}

func (s *PeerService) GetOnlinePeers() []*discovery.Peer {
	allPeers := s.discoveryMgr.GetPeers()
	online := make([]*discovery.Peer, 0)
	for _, p := range allPeers {
		if p.Online {
			online = append(online, p)
		}
	}
	return online
}

func (s *PeerService) GetPeerCount() int {
	return len(s.discoveryMgr.GetPeers())
}

func (s *PeerService) GetOnlineCount() int {
	return len(s.GetOnlinePeers())
}

func (s *PeerService) FormatLastSeen(t time.Time) string {
	duration := time.Since(t)

	switch {
	case duration < time.Minute:
		return "just now"
	case duration < time.Hour:
		return fmt.Sprintf("%d minutes ago", int(duration.Minutes()))
	case duration < 24*time.Hour:
		return fmt.Sprintf("%d hours ago", int(duration.Hours()))
	default:
		return fmt.Sprintf("%d days ago", int(duration.Hours()/24))
	}
}

func (s *PeerService) OnPeerFound(callback func(*discovery.Peer)) {
	s.discoveryMgr.OnPeerFound(callback)
}

func (s *PeerService) OnPeerLost(callback func(string)) {
	s.discoveryMgr.OnPeerLost(callback)
}
