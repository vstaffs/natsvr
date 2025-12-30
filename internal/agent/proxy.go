package agent

import (
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"

	"github.com/natsvr/natsvr/internal/protocol"
)

// LocalProxy manages local port forwarding
type LocalProxy struct {
	client      *Client
	listenPort  int
	targetAgent string
	targetHost  string
	targetPort  int
	protocol    string
	listener    net.Listener
	udpConn     *net.UDPConn
	running     bool
	runMu       sync.Mutex
	tunnelIDGen uint32
}

// NewLocalProxy creates a new local proxy
func NewLocalProxy(client *Client, listenPort int, targetAgent, targetHost string, targetPort int, proto string) *LocalProxy {
	return &LocalProxy{
		client:      client,
		listenPort:  listenPort,
		targetAgent: targetAgent,
		targetHost:  targetHost,
		targetPort:  targetPort,
		protocol:    proto,
	}
}

// Start starts the local proxy
func (p *LocalProxy) Start() error {
	p.runMu.Lock()
	defer p.runMu.Unlock()

	if p.running {
		return fmt.Errorf("proxy already running")
	}

	switch p.protocol {
	case "tcp":
		listener, err := net.Listen("tcp", fmt.Sprintf(":%d", p.listenPort))
		if err != nil {
			return err
		}
		p.listener = listener
		go p.acceptTCP()

	case "udp":
		addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", p.listenPort))
		if err != nil {
			return err
		}
		conn, err := net.ListenUDP("udp", addr)
		if err != nil {
			return err
		}
		p.udpConn = conn
		go p.handleUDP()
	}

	p.running = true
	log.Printf("Local proxy started on :%d -> %s:%s:%d", p.listenPort, p.targetAgent, p.targetHost, p.targetPort)

	return nil
}

// Stop stops the local proxy
func (p *LocalProxy) Stop() {
	p.runMu.Lock()
	defer p.runMu.Unlock()

	p.running = false
	if p.listener != nil {
		p.listener.Close()
	}
	if p.udpConn != nil {
		p.udpConn.Close()
	}
}

func (p *LocalProxy) acceptTCP() {
	for {
		p.runMu.Lock()
		running := p.running
		p.runMu.Unlock()

		if !running {
			return
		}

		conn, err := p.listener.Accept()
		if err != nil {
			if p.running {
				log.Printf("Accept error: %v", err)
			}
			continue
		}

		go p.handleTCPConn(conn)
	}
}

func (p *LocalProxy) handleTCPConn(conn net.Conn) {
	defer conn.Close()

	tunnelID := atomic.AddUint32(&p.tunnelIDGen, 1)

	// Request tunnel through agent -> cloud -> target agent
	// This would need to send a special message type that the cloud
	// routes to the target agent

	// For local forwarding, we just connect to the local target
	target, err := net.Dial("tcp", fmt.Sprintf("%s:%d", p.targetHost, p.targetPort))
	if err != nil {
		log.Printf("Failed to connect to target: %v", err)
		return
	}
	defer target.Close()

	log.Printf("Local proxy connection %d established", tunnelID)

	// Bidirectional copy
	done := make(chan struct{}, 2)

	go func() {
		buf := make([]byte, 32768)
		for {
			n, err := conn.Read(buf)
			if err != nil {
				done <- struct{}{}
				return
			}
			if _, err := target.Write(buf[:n]); err != nil {
				done <- struct{}{}
				return
			}
		}
	}()

	go func() {
		buf := make([]byte, 32768)
		for {
			n, err := target.Read(buf)
			if err != nil {
				done <- struct{}{}
				return
			}
			if _, err := conn.Write(buf[:n]); err != nil {
				done <- struct{}{}
				return
			}
		}
	}()

	<-done
	log.Printf("Local proxy connection %d closed", tunnelID)
}

func (p *LocalProxy) handleUDP() {
	buf := make([]byte, 65535)
	clients := make(map[string]*net.UDPAddr)

	for {
		p.runMu.Lock()
		running := p.running
		p.runMu.Unlock()

		if !running {
			return
		}

		n, addr, err := p.udpConn.ReadFromUDP(buf)
		if err != nil {
			continue
		}

		clients[addr.String()] = addr

		// Forward to target
		targetAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", p.targetHost, p.targetPort))
		if err != nil {
			continue
		}

		targetConn, err := net.DialUDP("udp", nil, targetAddr)
		if err != nil {
			continue
		}

		targetConn.Write(buf[:n])
		targetConn.Close()
	}
}

// RemoteProxy handles remote forwarding (Cloud -> Agent -> Local Service)
type RemoteProxy struct {
	client   *Client
	tunnels  map[uint32]*RemoteTunnelConn
	tunnelMu sync.RWMutex
}

// RemoteTunnelConn represents a remote tunnel connection
type RemoteTunnelConn struct {
	TunnelID   uint32
	LocalConn  net.Conn
	TargetHost string
	TargetPort int
}

// NewRemoteProxy creates a new remote proxy
func NewRemoteProxy(client *Client) *RemoteProxy {
	return &RemoteProxy{
		client:  client,
		tunnels: make(map[uint32]*RemoteTunnelConn),
	}
}

// HandleConnect handles a remote connect request
func (p *RemoteProxy) HandleConnect(tunnelID uint32, targetHost string, targetPort int) error {
	addr := fmt.Sprintf("%s:%d", targetHost, targetPort)
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return err
	}

	tunnel := &RemoteTunnelConn{
		TunnelID:   tunnelID,
		LocalConn:  conn,
		TargetHost: targetHost,
		TargetPort: targetPort,
	}

	p.tunnelMu.Lock()
	p.tunnels[tunnelID] = tunnel
	p.tunnelMu.Unlock()

	// Start reading from local connection
	go p.readFromLocal(tunnel)

	return nil
}

func (p *RemoteProxy) readFromLocal(tunnel *RemoteTunnelConn) {
	defer func() {
		tunnel.LocalConn.Close()
		p.tunnelMu.Lock()
		delete(p.tunnels, tunnel.TunnelID)
		p.tunnelMu.Unlock()

		// Send close message
		msg := protocol.NewCloseMessage(tunnel.TunnelID)
		p.client.sendMessage(msg)
	}()

	buf := make([]byte, 32768)
	for {
		n, err := tunnel.LocalConn.Read(buf)
		if err != nil {
			return
		}

		msg := protocol.NewDataMessage(tunnel.TunnelID, buf[:n])
		if err := p.client.sendMessage(msg); err != nil {
			return
		}
	}
}

// HandleData handles incoming data for a remote tunnel
func (p *RemoteProxy) HandleData(tunnelID uint32, data []byte) error {
	p.tunnelMu.RLock()
	tunnel, exists := p.tunnels[tunnelID]
	p.tunnelMu.RUnlock()

	if !exists {
		return fmt.Errorf("tunnel %d not found", tunnelID)
	}

	_, err := tunnel.LocalConn.Write(data)
	return err
}

// HandleClose handles a tunnel close message
func (p *RemoteProxy) HandleClose(tunnelID uint32) {
	p.tunnelMu.Lock()
	tunnel, exists := p.tunnels[tunnelID]
	if exists {
		tunnel.LocalConn.Close()
		delete(p.tunnels, tunnelID)
	}
	p.tunnelMu.Unlock()
}

