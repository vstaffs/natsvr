package cloud

import (
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/natsvr/natsvr/internal/protocol"
)

// Forwarder manages port forwarding rules
type Forwarder struct {
	server       *Server
	rules        map[string]*ForwardRuleState
	rulesMu      sync.RWMutex
	tunnelIDGen  uint32
	tunnelConns  map[uint32]*TunnelConn
	tunnelConnMu sync.RWMutex
	pendingAcks  map[uint32]chan *protocol.ConnectAckPayload
	pendingMu    sync.Mutex
	globalStats  *GlobalStats
}

// ForwardRuleState holds the runtime state of a forwarding rule
type ForwardRuleState struct {
	Rule        *ForwardRule
	Listener    net.Listener
	UDPConn     *net.UDPConn
	Active      bool
	RateLimiter *RateLimiter
	TrafficUsed int64 // atomic
}

// TunnelConn represents an active tunnel connection
type TunnelConn struct {
	ID       uint32
	AgentID  string
	Conn     net.Conn
	Protocol string
	Target   string
}

// NewForwarder creates a new forwarder
func NewForwarder(server *Server) *Forwarder {
	return &Forwarder{
		server:      server,
		rules:       make(map[string]*ForwardRuleState),
		tunnelConns: make(map[uint32]*TunnelConn),
		pendingAcks: make(map[uint32]chan *protocol.ConnectAckPayload),
		globalStats: NewGlobalStats(),
	}
}

// GetGlobalStats returns the global traffic statistics
func (f *Forwarder) GetGlobalStats() (txBytes, rxBytes int64, txSpeed, rxSpeed float64) {
	return f.globalStats.GetStats()
}

// GetRuleTraffic returns traffic used for a specific rule
func (f *Forwarder) GetRuleTraffic(ruleID string) int64 {
	f.rulesMu.RLock()
	defer f.rulesMu.RUnlock()
	if state, ok := f.rules[ruleID]; ok {
		return atomic.LoadInt64(&state.TrafficUsed)
	}
	return 0
}

// Run starts the forwarder
func (f *Forwarder) Run() {
	// Load existing rules from store
	rules, err := f.server.store.GetForwardRules()
	if err != nil {
		log.Printf("Failed to load forward rules: %v", err)
		return
	}

	for _, rule := range rules {
		if rule.Enabled {
			f.StartRule(rule)
		}
	}
}

// StartRule starts a forwarding rule
func (f *Forwarder) StartRule(rule *ForwardRule) error {
	f.rulesMu.Lock()
	defer f.rulesMu.Unlock()

	if _, exists := f.rules[rule.ID]; exists {
		return fmt.Errorf("rule %s already running", rule.ID)
	}

	state := &ForwardRuleState{
		Rule:        rule,
		Active:      true,
		RateLimiter: NewRateLimiter(rule.RateLimit),
		TrafficUsed: rule.TrafficUsed,
	}

	switch rule.Type {
	case "remote":
		// Cloud listens, forwards to agent
		if rule.Protocol == "tcp" {
			listener, err := net.Listen("tcp", fmt.Sprintf(":%d", rule.ListenPort))
			if err != nil {
				return fmt.Errorf("failed to listen on port %d: %v", rule.ListenPort, err)
			}
			state.Listener = listener
			go f.handleRemoteTCPListener(state)
		} else if rule.Protocol == "udp" {
			addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", rule.ListenPort))
			if err != nil {
				return err
			}
			conn, err := net.ListenUDP("udp", addr)
			if err != nil {
				return fmt.Errorf("failed to listen on UDP port %d: %v", rule.ListenPort, err)
			}
			state.UDPConn = conn
			go f.handleRemoteUDPListener(state)
		}
	case "cloud-self":
		// Cloud listens, forwards directly to target server (no agent involved)
		if rule.Protocol == "tcp" {
			listener, err := net.Listen("tcp", fmt.Sprintf(":%d", rule.ListenPort))
			if err != nil {
				return fmt.Errorf("failed to listen on port %d: %v", rule.ListenPort, err)
			}
			state.Listener = listener
			go f.handleCloudSelfTCPListener(state)
		} else if rule.Protocol == "udp" {
			addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", rule.ListenPort))
			if err != nil {
				return err
			}
			conn, err := net.ListenUDP("udp", addr)
			if err != nil {
				return fmt.Errorf("failed to listen on UDP port %d: %v", rule.ListenPort, err)
			}
			state.UDPConn = conn
			go f.handleCloudSelfUDPListener(state)
		}
	case "local", "p2p":
		// Agent to agent forwarding - handled when agents connect
	}

	f.rules[rule.ID] = state
	log.Printf("Started forward rule: %s (%s:%d -> %s:%s:%d)",
		rule.Name, rule.Protocol, rule.ListenPort,
		rule.TargetAgentID, rule.TargetHost, rule.TargetPort)

	return nil
}

// StopRule stops a forwarding rule
func (f *Forwarder) StopRule(ruleID string) error {
	f.rulesMu.Lock()
	defer f.rulesMu.Unlock()

	state, exists := f.rules[ruleID]
	if !exists {
		return nil
	}

	state.Active = false
	if state.Listener != nil {
		state.Listener.Close()
	}
	if state.UDPConn != nil {
		state.UDPConn.Close()
	}

	// Save traffic used to database
	trafficUsed := atomic.LoadInt64(&state.TrafficUsed)
	f.server.store.UpdateTrafficUsed(ruleID, trafficUsed)

	delete(f.rules, ruleID)
	log.Printf("Stopped forward rule: %s", state.Rule.Name)

	return nil
}

// addTraffic adds traffic to rule state and checks limits
// Returns false if traffic limit exceeded
func (f *Forwarder) addTraffic(state *ForwardRuleState, n int64) bool {
	newTotal := atomic.AddInt64(&state.TrafficUsed, n)
	f.globalStats.AddTx(n)
	
	// Check traffic limit
	if state.Rule.TrafficLimit > 0 && newTotal > state.Rule.TrafficLimit {
		return false
	}
	return true
}

func (f *Forwarder) handleRemoteTCPListener(state *ForwardRuleState) {
	for state.Active {
		conn, err := state.Listener.Accept()
		if err != nil {
			if state.Active {
				log.Printf("Accept error: %v", err)
			}
			continue
		}

		go f.handleRemoteTCPConnection(state, conn)
	}
}

func (f *Forwarder) handleRemoteTCPConnection(state *ForwardRuleState, conn net.Conn) {
	defer conn.Close()

	// Check traffic limit before starting
	if state.Rule.TrafficLimit > 0 && atomic.LoadInt64(&state.TrafficUsed) >= state.Rule.TrafficLimit {
		log.Printf("Traffic limit exceeded for rule %s", state.Rule.Name)
		return
	}

	rule := state.Rule
	agent := f.server.GetAgent(rule.TargetAgentID)
	if agent == nil {
		log.Printf("Target agent %s not connected", rule.TargetAgentID)
		return
	}

	// Generate tunnel ID
	tunnelID := atomic.AddUint32(&f.tunnelIDGen, 1)

	// Create pending ack channel
	ackChan := make(chan *protocol.ConnectAckPayload, 1)
	f.pendingMu.Lock()
	f.pendingAcks[tunnelID] = ackChan
	f.pendingMu.Unlock()

	defer func() {
		f.pendingMu.Lock()
		delete(f.pendingAcks, tunnelID)
		f.pendingMu.Unlock()
	}()

	// Send connect request to agent
	connectMsg := protocol.NewConnectMessage(tunnelID, "tcp", rule.TargetHost, uint16(rule.TargetPort))
	if err := f.server.sendToAgent(agent, connectMsg); err != nil {
		log.Printf("Failed to send connect message: %v", err)
		return
	}

	// Wait for acknowledgment
	select {
	case ack := <-ackChan:
		if !ack.Success {
			log.Printf("Tunnel connect failed: %s", ack.Error)
			return
		}
	case <-time.After(30 * time.Second):
		log.Printf("Tunnel connect timeout")
		return
	}

	// Register tunnel connection
	tunnelConn := &TunnelConn{
		ID:       tunnelID,
		AgentID:  agent.ID,
		Conn:     conn,
		Protocol: "tcp",
		Target:   fmt.Sprintf("%s:%d", rule.TargetHost, rule.TargetPort),
	}

	f.tunnelConnMu.Lock()
	f.tunnelConns[tunnelID] = tunnelConn
	f.tunnelConnMu.Unlock()

	agent.tunnelsMu.Lock()
	agent.tunnels[tunnelID] = &Tunnel{
		ID:         tunnelID,
		Protocol:   "tcp",
		TargetHost: rule.TargetHost,
		TargetPort: uint16(rule.TargetPort),
		CreatedAt:  time.Now(),
	}
	agent.ActiveTunnels++
	agent.tunnelsMu.Unlock()

	defer func() {
		f.tunnelConnMu.Lock()
		delete(f.tunnelConns, tunnelID)
		f.tunnelConnMu.Unlock()

		agent.tunnelsMu.Lock()
		delete(agent.tunnels, tunnelID)
		agent.ActiveTunnels--
		agent.tunnelsMu.Unlock()

		// Send close message
		f.server.sendToAgent(agent, protocol.NewCloseMessage(tunnelID))
	}()

	// Forward data from client to agent
	buf := make([]byte, 32768)
	for state.Active {
		n, err := conn.Read(buf)
		if err != nil {
			return
		}

		if n > 0 {
			// Check traffic limit
			if !f.addTraffic(state, int64(n)) {
				log.Printf("Traffic limit exceeded for rule %s", state.Rule.Name)
				return
			}

			// Apply rate limit
			if state.RateLimiter != nil {
				state.RateLimiter.Wait(int64(n))
			}

			dataMsg := protocol.NewDataMessage(tunnelID, buf[:n])
			if err := f.server.sendToAgent(agent, dataMsg); err != nil {
				return
			}
		}
	}
}

func (f *Forwarder) handleRemoteUDPListener(state *ForwardRuleState) {
	rule := state.Rule
	buf := make([]byte, 65535)
	clients := make(map[string]time.Time)

	for state.Active {
		state.UDPConn.SetReadDeadline(time.Now().Add(time.Second))
		n, addr, err := state.UDPConn.ReadFromUDP(buf)
		if err != nil {
			continue
		}

		agent := f.server.GetAgent(rule.TargetAgentID)
		if agent == nil {
			continue
		}

		clients[addr.String()] = time.Now()

		// Send UDP data to agent
		payload := protocol.EncodeUDPDataPayload(&protocol.UDPDataPayload{
			SourceAddr: addr.IP.String(),
			SourcePort: uint16(addr.Port),
			DestAddr:   rule.TargetHost,
			DestPort:   uint16(rule.TargetPort),
			Data:       buf[:n],
		})

		msg := protocol.NewMessage(protocol.MsgTypeUDPData, 0, payload)
		f.server.sendToAgent(agent, msg)
	}
}

// handleCloudSelfTCPListener handles TCP connections for cloud-self forwarding
func (f *Forwarder) handleCloudSelfTCPListener(state *ForwardRuleState) {
	for state.Active {
		conn, err := state.Listener.Accept()
		if err != nil {
			if state.Active {
				log.Printf("Accept error: %v", err)
			}
			continue
		}

		go f.handleCloudSelfTCPConnection(state, conn)
	}
}

// handleCloudSelfTCPConnection handles a single TCP connection for cloud-self forwarding
func (f *Forwarder) handleCloudSelfTCPConnection(state *ForwardRuleState, clientConn net.Conn) {
	defer clientConn.Close()

	// Check traffic limit before starting
	if state.Rule.TrafficLimit > 0 && atomic.LoadInt64(&state.TrafficUsed) >= state.Rule.TrafficLimit {
		log.Printf("Traffic limit exceeded for rule %s", state.Rule.Name)
		return
	}

	rule := state.Rule
	targetAddr := fmt.Sprintf("%s:%d", rule.TargetHost, rule.TargetPort)

	// Connect to target server directly
	targetConn, err := net.DialTimeout("tcp", targetAddr, 30*time.Second)
	if err != nil {
		log.Printf("Failed to connect to target %s: %v", targetAddr, err)
		return
	}
	defer targetConn.Close()

	// Bidirectional copy with rate limiting and traffic tracking
	done := make(chan struct{}, 2)

	// Client -> Target
	go func() {
		f.copyWithLimits(state, targetConn, clientConn)
		done <- struct{}{}
	}()

	// Target -> Client
	go func() {
		f.copyWithLimits(state, clientConn, targetConn)
		done <- struct{}{}
	}()

	// Wait for either direction to complete
	<-done
}

// copyWithLimits copies data with rate limiting and traffic tracking
func (f *Forwarder) copyWithLimits(state *ForwardRuleState, dst io.Writer, src io.Reader) {
	buf := make([]byte, 32768)
	for state.Active {
		n, err := src.Read(buf)
		if err != nil {
			return
		}
		if n > 0 {
			// Check traffic limit
			if !f.addTraffic(state, int64(n)) {
				log.Printf("Traffic limit exceeded for rule %s", state.Rule.Name)
				return
			}
			
			// Apply rate limit
			if state.RateLimiter != nil {
				state.RateLimiter.Wait(int64(n))
			}
			
			_, err = dst.Write(buf[:n])
			if err != nil {
				return
			}
		}
	}
}

// handleCloudSelfUDPListener handles UDP packets for cloud-self forwarding
func (f *Forwarder) handleCloudSelfUDPListener(state *ForwardRuleState) {
	rule := state.Rule
	buf := make([]byte, 65535)
	targetAddr := fmt.Sprintf("%s:%d", rule.TargetHost, rule.TargetPort)

	// Map to track client connections
	clients := make(map[string]*net.UDPConn)
	clientsMu := sync.Mutex{}

	for state.Active {
		state.UDPConn.SetReadDeadline(time.Now().Add(time.Second))
		n, clientAddr, err := state.UDPConn.ReadFromUDP(buf)
		if err != nil {
			continue
		}

		clientKey := clientAddr.String()

		clientsMu.Lock()
		targetConn, exists := clients[clientKey]
		if !exists {
			// Create new connection to target
			raddr, err := net.ResolveUDPAddr("udp", targetAddr)
			if err != nil {
				clientsMu.Unlock()
				log.Printf("Failed to resolve target address %s: %v", targetAddr, err)
				continue
			}
			targetConn, err = net.DialUDP("udp", nil, raddr)
			if err != nil {
				clientsMu.Unlock()
				log.Printf("Failed to connect to target %s: %v", targetAddr, err)
				continue
			}
			clients[clientKey] = targetConn

			// Start goroutine to receive responses
			go func(clientAddr *net.UDPAddr, targetConn *net.UDPConn) {
				respBuf := make([]byte, 65535)
				for {
					targetConn.SetReadDeadline(time.Now().Add(30 * time.Second))
					n, err := targetConn.Read(respBuf)
					if err != nil {
						clientsMu.Lock()
						delete(clients, clientAddr.String())
						clientsMu.Unlock()
						targetConn.Close()
						return
					}
					state.UDPConn.WriteToUDP(respBuf[:n], clientAddr)
				}
			}(clientAddr, targetConn)
		}
		clientsMu.Unlock()

		// Forward data to target
		targetConn.Write(buf[:n])
	}

	// Cleanup all connections
	clientsMu.Lock()
	for _, conn := range clients {
		conn.Close()
	}
	clientsMu.Unlock()
}

// HandleData handles incoming data from an agent
func (f *Forwarder) HandleData(agent *AgentConn, msg *protocol.Message) {
	f.tunnelConnMu.RLock()
	tunnelConn, exists := f.tunnelConns[msg.TunnelID]
	f.tunnelConnMu.RUnlock()

	if !exists {
		return
	}

	_, err := tunnelConn.Conn.Write(msg.Payload)
	if err != nil {
		tunnelConn.Conn.Close()
	}
}

// HandleUDPData handles incoming UDP data from an agent
func (f *Forwarder) HandleUDPData(agent *AgentConn, msg *protocol.Message) {
	payload, err := protocol.DecodeUDPDataPayload(msg.Payload)
	if err != nil {
		return
	}

	// Find the rule that matches this response
	f.rulesMu.RLock()
	for _, state := range f.rules {
		if state.Rule.TargetAgentID == agent.ID && state.UDPConn != nil {
			addr, _ := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", payload.DestAddr, payload.DestPort))
			state.UDPConn.WriteToUDP(payload.Data, addr)
			break
		}
	}
	f.rulesMu.RUnlock()
}

// HandleICMPData handles incoming ICMP data from an agent
func (f *Forwarder) HandleICMPData(agent *AgentConn, msg *protocol.Message) {
	// ICMP handling - requires raw sockets, typically needs root privileges
	log.Printf("Received ICMP data from agent %s", agent.ID)
}

// HandleConnectAck handles tunnel connect acknowledgment
func (f *Forwarder) HandleConnectAck(agent *AgentConn, msg *protocol.Message) {
	ack, err := protocol.DecodeConnectAckPayload(msg.Payload)
	if err != nil {
		return
	}

	f.pendingMu.Lock()
	ch, exists := f.pendingAcks[msg.TunnelID]
	f.pendingMu.Unlock()

	if exists {
		ch <- ack
	}
}

// HandleClose handles tunnel close message
func (f *Forwarder) HandleClose(agent *AgentConn, msg *protocol.Message) {
	f.tunnelConnMu.Lock()
	tunnelConn, exists := f.tunnelConns[msg.TunnelID]
	if exists {
		tunnelConn.Conn.Close()
		delete(f.tunnelConns, msg.TunnelID)
	}
	f.tunnelConnMu.Unlock()

	agent.tunnelsMu.Lock()
	if _, ok := agent.tunnels[msg.TunnelID]; ok {
		delete(agent.tunnels, msg.TunnelID)
		agent.ActiveTunnels--
	}
	agent.tunnelsMu.Unlock()
}

// SendToAgent sends a message to an agent
func (f *Forwarder) SendToAgent(agentID string, msg *protocol.Message) error {
	agent := f.server.GetAgent(agentID)
	if agent == nil {
		return fmt.Errorf("agent %s not found", agentID)
	}

	data, err := msg.Encode()
	if err != nil {
		return err
	}

	agent.writeMu.Lock()
	defer agent.writeMu.Unlock()

	return agent.Conn.WriteMessage(websocket.BinaryMessage, data)
}

