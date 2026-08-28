package gateway

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ForwardRule defines a stream proxy routing rule from gateway listen port to target worker host:port or tunnel
type ForwardRule struct {
	ID         string
	NodeID     string // Associated compute node ID
	ListenPort int
	TargetHost string
	TargetPort int
	Protocol   string // "tcp" or "udp"
}

// TunnelRequest represents a pending connection request waiting for the agent to connect back
type TunnelRequest struct {
	StreamID   string `json:"streamId"`
	NodeID     string `json:"nodeId"`
	ServerID   string `json:"serverId"`
	TargetPort int    `json:"targetPort"`
}

type pendingClient struct {
	clientConn net.Conn
	ready      chan net.Conn
	done       chan struct{}
}

// StreamGateway manages dynamic TCP/UDP port forwardings and reverse tunnel multiplexing for remote/NAT nodes
type StreamGateway struct {
	mu             sync.RWMutex
	listeners      map[int]net.Listener
	rules          map[string]ForwardRule
	pendingClients map[string]*pendingClient     // streamID -> pendingClient
	nodeRequests   map[string]chan TunnelRequest // nodeID -> channel of pending requests
	logger         *slog.Logger
	ctx            context.Context
	cancel         context.CancelFunc
}

func NewStreamGateway(logger *slog.Logger) *StreamGateway {
	ctx, cancel := context.WithCancel(context.Background())
	return &StreamGateway{
		listeners:      make(map[int]net.Listener),
		rules:          make(map[string]ForwardRule),
		pendingClients: make(map[string]*pendingClient),
		nodeRequests:   make(map[string]chan TunnelRequest),
		logger:         logger,
		ctx:            ctx,
		cancel:         cancel,
	}
}

// SubscribeNodeRequests returns a channel that receives tunnel requests for a given node
func (g *StreamGateway) SubscribeNodeRequests(nodeID string) <-chan TunnelRequest {
	g.mu.Lock()
	defer g.mu.Unlock()
	ch, ok := g.nodeRequests[nodeID]
	if !ok {
		ch = make(chan TunnelRequest, 64)
		g.nodeRequests[nodeID] = ch
	}
	return ch
}

// UnsubscribeNodeRequests closes and removes the node request channel
func (g *StreamGateway) UnsubscribeNodeRequests(nodeID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if ch, ok := g.nodeRequests[nodeID]; ok {
		delete(g.nodeRequests, nodeID)
		close(ch)
	}
}

func (g *StreamGateway) IsListening(port int) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	_, ok := g.listeners[port]
	return ok
}

// RegisterForward registers and starts a TCP proxy listener on listenPort forwarding to targetHost:targetPort
func (g *StreamGateway) RegisterForward(rule ForwardRule) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	// If already listening on this port for the same target and node, ignore
	if existing, ok := g.rules[rule.ID]; ok && existing.ListenPort == rule.ListenPort && existing.TargetHost == rule.TargetHost && existing.TargetPort == rule.TargetPort && existing.NodeID == rule.NodeID {
		return nil
	}

	// Close old listener if rule exists
	if _, ok := g.rules[rule.ID]; ok {
		g.unregisterLocked(rule.ID)
	}

	if (rule.NodeID == "" || rule.NodeID == "node-local") && (rule.TargetHost == "" || rule.TargetHost == "0.0.0.0" || rule.TargetHost == "127.0.0.1") {
		// Target is local host daemon; Docker on host already binds the hostPort directly
		return nil
	}

	addr := net.JoinHostPort("0.0.0.0", strconv.Itoa(rule.ListenPort))
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		g.logger.Warn("stream gateway failed to bind port", "port", rule.ListenPort, "node", rule.NodeID, "error", err)
		return err
	}

	g.listeners[rule.ListenPort] = listener
	g.rules[rule.ID] = rule

	g.logger.Info("stream gateway registered forward proxy",
		"rule_id", rule.ID,
		"node_id", rule.NodeID,
		"listen_port", rule.ListenPort,
		"target_port", rule.TargetPort,
	)

	go g.serveTCP(listener, rule)
	return nil
}

func (g *StreamGateway) UnregisterForward(ruleID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.unregisterLocked(ruleID)
}

func (g *StreamGateway) unregisterLocked(ruleID string) {
	rule, ok := g.rules[ruleID]
	if !ok {
		return
	}
	if l, ok := g.listeners[rule.ListenPort]; ok {
		_ = l.Close()
		delete(g.listeners, rule.ListenPort)
	}
	delete(g.rules, ruleID)
	g.logger.Info("stream gateway removed forward proxy", "rule_id", ruleID, "port", rule.ListenPort)
}

func (g *StreamGateway) serveTCP(l net.Listener, rule ForwardRule) {
	for {
		conn, err := l.Accept()
		if err != nil {
			select {
			case <-g.ctx.Done():
				return
			default:
				return
			}
		}

		go g.pipeTCP(conn, rule)
	}
}

func (g *StreamGateway) pipeTCP(clientConn net.Conn, rule ForwardRule) {
	defer clientConn.Close()
	clientRemote := clientConn.RemoteAddr().String()

	g.logger.Info("gateway accepted incoming client connection",
		"remote_addr", clientRemote,
		"listen_port", rule.ListenPort,
		"node_id", rule.NodeID,
	)

	// 1. If target host is publicly dialable, try direct dial first
	if rule.TargetHost != "" && rule.TargetHost != "0.0.0.0" && rule.TargetHost != "127.0.0.1" && rule.TargetHost != "localhost" {
		targetAddr := net.JoinHostPort(rule.TargetHost, strconv.Itoa(rule.TargetPort))
		targetConn, err := net.DialTimeout("tcp", targetAddr, 3*time.Second)
		if err == nil {
			defer targetConn.Close()
			g.logger.Info("gateway direct dial connected to target host", "target", targetAddr)
			g.bidirectionalCopy(clientConn, targetConn)
			return
		}
		g.logger.Warn("direct dial to target failed, falling back to reverse tunnel", "target", targetAddr, "error", err)
	}

	// 2. Reverse Tunnel fallback (NAT-traversal)
	if rule.NodeID == "" || rule.NodeID == "node-local" {
		g.logger.Warn("cannot forward to local node without direct bind", "port", rule.ListenPort)
		return
	}

	streamID := fmt.Sprintf("strm_%s", uuid.NewString()[:8])
	pending := &pendingClient{
		clientConn: clientConn,
		ready:      make(chan net.Conn, 1),
		done:       make(chan struct{}),
	}

	g.mu.Lock()
	g.pendingClients[streamID] = pending
	reqChan, hasSub := g.nodeRequests[rule.NodeID]
	g.mu.Unlock()

	defer func() {
		g.mu.Lock()
		delete(g.pendingClients, streamID)
		g.mu.Unlock()
		close(pending.done)
	}()

	if !hasSub || reqChan == nil {
		g.logger.Warn("no agent connected to reverse tunnel for node", "node_id", rule.NodeID, "stream_id", streamID)
		return
	}

	req := TunnelRequest{
		StreamID:   streamID,
		NodeID:     rule.NodeID,
		ServerID:   rule.ID,
		TargetPort: rule.TargetPort,
	}

	g.logger.Info("gateway dispatching tunnel request to agent", "port", rule.ListenPort, "node_id", rule.NodeID, "stream_id", streamID)

	select {
	case reqChan <- req:
		g.logger.Info("dispatched tunnel request to agent channel successfully", "stream_id", streamID, "node_id", rule.NodeID)
	case <-time.After(3 * time.Second):
		g.logger.Warn("timed out dispatching tunnel request to agent channel", "stream_id", streamID, "node_id", rule.NodeID)
		return
	}

	// Wait up to 10 seconds for agent to connect back
	select {
	case agentConn := <-pending.ready:
		if agentConn == nil {
			g.logger.Warn("agent reverse tunnel connection returned nil", "stream_id", streamID)
			return
		}
		defer agentConn.Close()
		g.logger.Info("reverse tunnel stream established, piping game traffic", "stream_id", streamID, "port", rule.ListenPort)
		g.bidirectionalCopy(clientConn, agentConn)
		g.logger.Info("reverse tunnel stream finished", "stream_id", streamID)
	case <-time.After(10 * time.Second):
		g.logger.Warn("timed out waiting for agent reverse tunnel connection", "stream_id", streamID)
	}
}

// HandleAgentTunnelConnect handles an incoming reverse tunnel connection from an agent
func (g *StreamGateway) HandleAgentTunnelConnect(w http.ResponseWriter, r *http.Request, streamID string) error {
	g.logger.Info("gateway received reverse tunnel connect request from agent", "stream_id", streamID, "remote_addr", r.RemoteAddr)

	g.mu.RLock()
	pending, ok := g.pendingClients[streamID]
	g.mu.RUnlock()

	if !ok || pending == nil {
		g.logger.Warn("stream not found or expired on tunnel connect", "stream_id", streamID)
		http.Error(w, "stream not found or expired", http.StatusNotFound)
		return errors.New("stream not found or expired")
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijacking not supported", http.StatusInternalServerError)
		return errors.New("hijacking not supported")
	}

	conn, bufrw, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return err
	}

	// Send 101 Switching Protocols
	_, _ = conn.Write([]byte("HTTP/1.1 101 Switching Protocols\r\nUpgrade: stream-tunnel\r\nConnection: Upgrade\r\n\r\n"))
	_ = bufrw.Flush()

	select {
	case pending.ready <- conn:
		g.logger.Info("agent tunnel hijacked socket passed to pending client ready channel", "stream_id", streamID)
		// Wait until stream finishes
		<-pending.done
		return nil
	default:
		_ = conn.Close()
		return errors.New("pending client already received or closed")
	}
}

func (g *StreamGateway) bidirectionalCopy(c1, c2 net.Conn) {
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(c1, c2)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(c2, c1)
		done <- struct{}{}
	}()
	<-done
}

func (g *StreamGateway) Close() {
	g.cancel()
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, l := range g.listeners {
		_ = l.Close()
	}
	g.listeners = make(map[int]net.Listener)
}
