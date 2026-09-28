package network

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"
)

type UDPConfig struct {
	Addr        string
	ReadTimeout time.Duration
}

type UDPServer struct {
	cfg       *UDPConfig
	conn      *net.UDPConn
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.RWMutex
	onMessage func([]byte, *net.UDPAddr)
}

func NewUDPServer(cfg *UDPConfig) *UDPServer {
	ctx, cancel := context.WithCancel(context.Background())
	return &UDPServer{
		cfg:    cfg,
		ctx:    ctx,
		cancel: cancel,
	}
}

func (s *UDPServer) Start() error {
	addr, err := net.ResolveUDPAddr("udp", s.cfg.Addr)
	if err != nil {
		return err
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return err
	}

	if err := setBroadcastOptions(conn); err != nil {
		conn.Close()
		return fmt.Errorf("failed to set broadcast options: %w", err)
	}

	s.conn = conn
	go s.readLoop()

	return nil
}

func (s *UDPServer) readLoop() {
	buf := make([]byte, 2048)
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			if s.cfg.ReadTimeout > 0 {
				s.conn.SetReadDeadline(time.Now().Add(s.cfg.ReadTimeout))
			}

			n, remoteAddr, err := s.conn.ReadFromUDP(buf)
			if err != nil {
				continue
			}

			data := make([]byte, n)
			copy(data, buf[:n])

			if s.onMessage != nil {
				s.onMessage(data, remoteAddr)
			}
		}
	}
}

func (s *UDPServer) SendTo(data []byte, addr *net.UDPAddr) error {
	s.mu.RLock()
	conn := s.conn
	s.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("UDP server not started")
	}

	_, err := conn.WriteTo(data, addr)
	return err
}

func (s *UDPServer) Broadcast(data []byte, port int) error {
	s.mu.RLock()
	conn := s.conn
	s.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("UDP server not started")
	}

	broadcastAddrs := getBroadcastAddresses(port)
	var lastErr error
	for _, addr := range broadcastAddrs {
		_, err := conn.WriteTo(data, addr)
		if err != nil {
			lastErr = err
		}
	}
	return lastErr
}

func getBroadcastAddresses(port int) []*net.UDPAddr {
	addrs := []*net.UDPAddr{
		{IP: net.IPv4bcast, Port: port},
	}

	interfaces, err := net.Interfaces()
	if err != nil {
		return addrs
	}

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagBroadcast == 0 {
			continue
		}

		addrList, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrList {
			if ipnet, ok := addr.(*net.IPNet); ok {
				if ip4 := ipnet.IP.To4(); ip4 != nil {
					broadcastIP := getBroadcastIP(ip4, ipnet.Mask)
					if broadcastIP != nil {
						addrs = append(addrs, &net.UDPAddr{IP: broadcastIP, Port: port})
					}
				}
			}
		}
	}

	return addrs
}

func getBroadcastIP(ip net.IP, mask net.IPMask) net.IP {
	if len(ip) != 4 || len(mask) != 4 {
		return nil
	}

	broadcast := make(net.IP, 4)
	for i := 0; i < 4; i++ {
		broadcast[i] = ip[i] | ^mask[i]
	}

	return broadcast
}

func (s *UDPServer) Stop() {
	s.cancel()
	if s.conn != nil {
		s.conn.Close()
	}
}

func (s *UDPServer) OnMessage(callback func([]byte, *net.UDPAddr)) {
	s.onMessage = callback
}

func (s *UDPServer) LocalAddr() *net.UDPAddr {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.conn == nil {
		return nil
	}
	return s.conn.LocalAddr().(*net.UDPAddr)
}

func SendUDPBroadcast(data []byte, port int) error {
	conn, err := net.DialUDP("udp", nil, &net.UDPAddr{
		IP:   net.IPv4bcast,
		Port: port,
	})
	if err != nil {
		return err
	}
	defer conn.Close()

	_, err = conn.Write(data)
	return err
}
