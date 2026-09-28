package network

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

type TCPConfig struct {
	Addr         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

type TCPConnection struct {
	conn    net.Conn
	reader  *bufio.Reader
	writer  *bufio.Writer
	mu      sync.Mutex
	cfg     *TCPConfig
	closed  bool
	closeMu sync.RWMutex
}

func NewTCPConnection(conn net.Conn, cfg *TCPConfig) *TCPConnection {
	return &TCPConnection{
		conn:   conn,
		reader: bufio.NewReaderSize(conn, 256*1024),
		writer: bufio.NewWriterSize(conn, 256*1024),
		cfg:    cfg,
	}
}

func (tc *TCPConnection) Send(data []byte) error {
	tc.closeMu.RLock()
	if tc.closed {
		tc.closeMu.RUnlock()
		return fmt.Errorf("connection closed")
	}
	tc.closeMu.RUnlock()

	tc.mu.Lock()
	defer tc.mu.Unlock()

	if tc.cfg.WriteTimeout > 0 {
		tc.conn.SetWriteDeadline(time.Now().Add(tc.cfg.WriteTimeout))
	}

	n, err := tc.writer.Write(data)
	if err != nil {
		return err
	}

	if n != len(data) {
		return fmt.Errorf("partial write: %d of %d bytes", n, len(data))
	}

	return tc.writer.Flush()
}

func (tc *TCPConnection) Read(buf []byte) (int, error) {
	tc.closeMu.RLock()
	if tc.closed {
		tc.closeMu.RUnlock()
		return 0, fmt.Errorf("connection closed")
	}
	tc.closeMu.RUnlock()

	if tc.cfg.ReadTimeout > 0 {
		tc.conn.SetReadDeadline(time.Now().Add(tc.cfg.ReadTimeout))
	}

	return tc.reader.Read(buf)
}

func (tc *TCPConnection) ReadFull(buf []byte) (int, error) {
	tc.closeMu.RLock()
	if tc.closed {
		tc.closeMu.RUnlock()
		return 0, fmt.Errorf("connection closed")
	}
	tc.closeMu.RUnlock()

	if tc.cfg.ReadTimeout > 0 {
		tc.conn.SetReadDeadline(time.Now().Add(tc.cfg.ReadTimeout))
	}

	return io.ReadFull(tc.reader, buf)
}

func (tc *TCPConnection) Close() error {
	tc.closeMu.Lock()
	defer tc.closeMu.Unlock()

	if tc.closed {
		return nil
	}

	tc.closed = true
	return tc.conn.Close()
}

func (tc *TCPConnection) RemoteAddr() net.Addr {
	return tc.conn.RemoteAddr()
}

func (tc *TCPConnection) LocalAddr() net.Addr {
	return tc.conn.LocalAddr()
}

type TCPServer struct {
	cfg          *TCPConfig
	listener     net.Listener
	connections  map[string]*TCPConnection
	mu           sync.RWMutex
	ctx          context.Context
	cancel       context.CancelFunc
	onConnect    func(*TCPConnection)
	onDisconnect func(string)
}

func NewTCPServer(cfg *TCPConfig) *TCPServer {
	ctx, cancel := context.WithCancel(context.Background())
	return &TCPServer{
		cfg:         cfg,
		connections: make(map[string]*TCPConnection),
		ctx:         ctx,
		cancel:      cancel,
	}
}

func (s *TCPServer) Start() error {
	addr, err := net.ResolveTCPAddr("tcp", s.cfg.Addr)
	if err != nil {
		return err
	}

	listener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return err
	}

	s.listener = listener
	go s.acceptLoop()

	return nil
}

func (s *TCPServer) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.ctx.Done():
				return
			default:
				continue
			}
		}

		tcpConn := NewTCPConnection(conn, s.cfg)
		remoteAddr := conn.RemoteAddr().String()

		s.mu.Lock()
		s.connections[remoteAddr] = tcpConn
		s.mu.Unlock()

		if s.onConnect != nil {
			s.onConnect(tcpConn)
		}
	}
}

func (s *TCPServer) handleConnection(addr string, conn *TCPConnection) {
	buf := make([]byte, 64*1024)
	for {
		_, err := conn.Read(buf)
		if err != nil {
			s.removeConnection(addr)
			return
		}
	}
}

func (s *TCPServer) removeConnection(addr string) {
	s.mu.Lock()
	conn := s.connections[addr]
	delete(s.connections, addr)
	s.mu.Unlock()

	if conn != nil {
		conn.Close()
		if s.onDisconnect != nil {
			s.onDisconnect(addr)
		}
	}
}

func (s *TCPServer) Stop() {
	s.cancel()
	if s.listener != nil {
		s.listener.Close()
	}

	s.mu.Lock()
	for addr, conn := range s.connections {
		conn.Close()
		delete(s.connections, addr)
	}
	s.mu.Unlock()
}

func (s *TCPServer) OnConnect(callback func(*TCPConnection)) {
	s.onConnect = callback
}

func (s *TCPServer) OnDisconnect(callback func(string)) {
	s.onDisconnect = callback
}

func (s *TCPServer) GetConnection(addr string) (*TCPConnection, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	conn, ok := s.connections[addr]
	return conn, ok
}

func DialTCP(addr string, cfg *TCPConfig) (*TCPConnection, error) {
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return nil, err
	}

	return NewTCPConnection(conn, cfg), nil
}
