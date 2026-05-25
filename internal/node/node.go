package node

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"meshnet/internal/protocol"
)

type Peer struct {
	ID       string
	Addr     string
	enc      *json.Encoder
	conn     net.Conn
	LastSeen time.Time
	LastPing time.Time
	Latency  time.Duration
	Healthy  bool
	mu       sync.Mutex
}

type Route struct {
	NodeID    string
	Addr      string
	Via       string
	Hops      int
	Direct    bool
	UpdatedAt time.Time
}

type Delivery struct {
	ID       string
	Target   string
	Payload  string
	Status   string
	Attempts int
	LastSent time.Time
}

type Options struct {
	KnownPeers map[string]string
	OnKnown    func(map[string]string)
	ListenAddr string
}

type Node struct {
	id         string
	addr       string
	listenAddr string
	peers      map[string]*Peer
	known      map[string]string
	routes     map[string]Route
	links      map[string]TopologyLink
	objects    map[string]protocol.ObjectBody
	seen       map[string]bool
	pending    map[string]Delivery
	trace      bool
	monitor    bool
	onKnown    func(map[string]string)
	mu         sync.RWMutex
}

func New(addr, id string) *Node {
	return NewWithOptions(addr, id, Options{})
}

func NewWithOptions(addr, id string, opts Options) *Node {
	if id == "" {
		id = randomID()
	}
	n := &Node{
		id:         id,
		addr:       addr,
		listenAddr: addr,
		peers:      map[string]*Peer{},
		known:      map[string]string{},
		routes:     map[string]Route{},
		links:      map[string]TopologyLink{},
		objects:    map[string]protocol.ObjectBody{},
		seen:       map[string]bool{},
		pending:    map[string]Delivery{},
		onKnown:    opts.OnKnown,
	}
	if opts.ListenAddr != "" {
		n.listenAddr = opts.ListenAddr
	}
	n.known[id] = addr
	n.routes[id] = Route{NodeID: id, Addr: addr, Hops: 0, Direct: true, UpdatedAt: time.Now()}
	for peerID, peerAddr := range opts.KnownPeers {
		if peerID != "" && peerID != id {
			n.known[peerID] = peerAddr
		}
	}
	return n
}

func (n *Node) ID() string   { return n.id }
func (n *Node) Addr() string { return n.addr }

func (n *Node) SetTrace(enabled bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.trace = enabled
}

func (n *Node) SetMonitor(enabled bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.monitor = enabled
}

func (n *Node) Listen() {
	ln, err := net.Listen("tcp", n.listenAddr)
	if err != nil {
		log.Fatalf("listen failed: %v", err)
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("accept failed: %v", err)
			continue
		}
		go n.handleConn(conn, false)
	}
}

func (n *Node) Connect(addr string) error {
	if addr == n.addr {
		return fmt.Errorf("cannot connect node to itself")
	}
	if n.isConnectedAddr(addr) {
		return nil
	}
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return err
	}
	go n.handleConn(conn, true)
	return nil
}

func (n *Node) handleConn(conn net.Conn, outbound bool) {
	enc := json.NewEncoder(conn)
	dec := json.NewDecoder(conn)

	if outbound {
		if err := writeFrame(enc, protocol.TypeHello, protocol.HelloBody{NodeID: n.id, Addr: n.addr}); err != nil {
			log.Printf("hello failed: %v", err)
			_ = conn.Close()
			return
		}
	}

	var peer *Peer
	for {
		var frame protocol.Frame
		if err := dec.Decode(&frame); err != nil {
			if peer != nil {
				fmt.Printf("\npeer %s disconnected\n> ", short(peer.ID))
			} else if err != io.EOF && !isQuietNetErr(err) {
				log.Printf("read failed: %v", err)
			}
			if peer != nil {
				n.removePeer(peer.ID)
			}
			_ = conn.Close()
			return
		}
		if frame.Version != "" && frame.Version != protocol.Version {
			log.Printf("ignored %s frame with version %s", frame.Type, frame.Version)
			continue
		}

		if frame.Type == protocol.TypeHello {
			var body protocol.HelloBody
			if protocol.DecodeBody(frame.Body, &body) != nil || body.NodeID == "" {
				continue
			}
			if body.NodeID == n.id {
				log.Printf("ignored self connection from %s", body.Addr)
				_ = conn.Close()
				return
			}
			if n.hasPeer(body.NodeID) {
				_ = conn.Close()
				return
			}
			peer = n.addPeer(body.NodeID, body.Addr, enc, conn)
			if !outbound {
				_ = writeFrame(enc, protocol.TypeHello, protocol.HelloBody{NodeID: n.id, Addr: n.addr})
			}
			fmt.Printf("\nconnected peer %s at %s\n> ", short(body.NodeID), body.Addr)
			n.broadcastPeers()
			continue
		}

		if peer == nil {
			continue
		}
		peer.touch()
		n.handleFrame(peer, frame)
	}
}

func isQuietNetErr(err error) bool {
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "use of closed network connection") ||
		strings.Contains(text, "forcibly closed by the remote host") ||
		strings.Contains(text, "wsarecv")
}

func (n *Node) handleFrame(peer *Peer, frame protocol.Frame) {
	switch frame.Type {
	case protocol.TypePeers:
		var body protocol.PeerListBody
		if protocol.DecodeBody(frame.Body, &body) == nil {
			n.learnPeers(body.Peers, peer.ID)
		}
	case protocol.TypePing:
		var body protocol.PingBody
		if protocol.DecodeBody(frame.Body, &body) != nil || n.markSeen(body.ID) {
			return
		}
		n.learnRoute(body.From, "", peer.ID, len(body.Path)+1, false)
		if body.Target == n.id {
			n.route(protocol.TypePong, protocol.PongBody{RouteHeader: newHeader(body.ID, n.id, body.From)}, peer.ID)
			return
		}
		body.Path = append(body.Path, n.id)
		n.route(protocol.TypePing, body, peer.ID)
	case protocol.TypePong:
		var body protocol.PongBody
		if protocol.DecodeBody(frame.Body, &body) != nil {
			return
		}
		n.learnRoute(body.From, "", peer.ID, len(body.Path)+1, false)
		if body.Target == n.id {
			fmt.Printf("\npong from %s for %s\n> ", short(body.From), body.ID)
			return
		}
		body.Path = append(body.Path, n.id)
		n.route(protocol.TypePong, body, peer.ID)
	case protocol.TypeSend:
		var body protocol.SendBody
		if protocol.DecodeBody(frame.Body, &body) != nil || n.markSeen(body.ID) {
			return
		}
		n.learnRoute(body.From, "", peer.ID, len(body.Path)+1, false)
		if body.Target == n.id {
			fmt.Printf("\nmessage from %s: %s\n> ", short(body.From), body.Payload)
			n.route(protocol.TypeAck, protocol.AckBody{RouteHeader: newHeader(body.ID, n.id, body.From)}, peer.ID)
			return
		}
		n.monitorSend(body)
		body.Path = append(body.Path, n.id)
		n.route(protocol.TypeSend, body, peer.ID)
	case protocol.TypeAck:
		var body protocol.AckBody
		if protocol.DecodeBody(frame.Body, &body) != nil {
			return
		}
		n.learnRoute(body.From, "", peer.ID, len(body.Path)+1, false)
		if body.Target == n.id {
			n.markDelivered(body.ID, body.From)
			fmt.Printf("\nack received from %s for %s\n> ", short(body.From), body.ID)
			return
		}
		body.Path = append(body.Path, n.id)
		n.route(protocol.TypeAck, body, peer.ID)
	case protocol.TypeHeartbeat:
		var body protocol.HeartbeatBody
		if protocol.DecodeBody(frame.Body, &body) != nil {
			return
		}
		peer.send(protocol.TypeHeartbeatOK, protocol.HeartbeatBody{ID: body.ID, From: n.id, Time: body.Time})
	case protocol.TypeHeartbeatOK:
		var body protocol.HeartbeatBody
		if protocol.DecodeBody(frame.Body, &body) != nil {
			return
		}
		peer.setHealthy(time.Since(time.UnixMilli(body.Time)))
	case protocol.TypeObjectPut:
		var body protocol.ObjectBody
		if protocol.DecodeBody(frame.Body, &body) != nil || n.markSeen(body.ID) {
			return
		}
		n.learnRoute(body.From, "", peer.ID, len(body.Path)+1, false)
		if body.Target == "" || body.Target == n.id {
			n.mu.Lock()
			n.objects[body.ID] = body
			n.mu.Unlock()
			fmt.Printf("\nstored object %s (%s) from %s\n> ", short(body.ID), body.Name, short(body.From))
		}
		body.Path = append(body.Path, n.id)
		n.route(protocol.TypeObjectPut, body, peer.ID)
	case protocol.TypeObjectGet:
		var body protocol.ObjectBody
		if protocol.DecodeBody(frame.Body, &body) != nil {
			return
		}
		seenID := body.RequestID
		if seenID == "" {
			seenID = body.ID + ":get:" + body.From
		}
		if n.markSeen(seenID) {
			return
		}
		n.learnRoute(body.From, "", peer.ID, len(body.Path)+1, false)
		if body.Target == n.id || body.Target == "" {
			if obj, ok := n.objectByID(body.ID); ok {
				obj.RouteHeader = newHeader(randomID(), n.id, body.From)
				n.route(protocol.TypeObjectFound, obj, peer.ID)
			}
		}
		if body.Target != n.id {
			body.Path = append(body.Path, n.id)
			n.route(protocol.TypeObjectGet, body, peer.ID)
		}
	case protocol.TypeObjectFound:
		var body protocol.ObjectBody
		if protocol.DecodeBody(frame.Body, &body) != nil {
			return
		}
		n.learnRoute(body.From, "", peer.ID, len(body.Path)+1, false)
		if body.Target == n.id {
			n.mu.Lock()
			n.objects[body.ID] = body
			n.mu.Unlock()
			fmt.Printf("\nobject %s (%s): %s\n> ", short(body.ID), body.Name, body.Content)
			return
		}
		body.Path = append(body.Path, n.id)
		n.route(protocol.TypeObjectFound, body, peer.ID)
	}
}

func newHeader(id, from, target string) protocol.RouteHeader {
	return protocol.RouteHeader{ID: id, From: from, Target: target, TTL: protocol.DefaultTTL, Path: []string{from}}
}

func (n *Node) addPeer(id, addr string, enc *json.Encoder, conn net.Conn) *Peer {
	n.mu.Lock()
	defer n.mu.Unlock()
	peer := &Peer{ID: id, Addr: addr, enc: enc, conn: conn, LastSeen: time.Now(), Healthy: true}
	n.peers[id] = peer
	n.known[id] = addr
	n.routes[id] = Route{NodeID: id, Addr: addr, Via: id, Hops: 1, Direct: true, UpdatedAt: time.Now()}
	n.links[linkKey(n.id, id)] = TopologyLink{From: n.id, To: id, Kind: "direct", Health: "healthy"}
	n.known[n.id] = n.addr
	n.notifyKnownLocked()
	return peer
}

func (n *Node) hasPeer(id string) bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.peers[id] != nil
}

func (n *Node) isConnectedAddr(addr string) bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	for _, peer := range n.peers {
		if peer.Addr == addr {
			return true
		}
	}
	return false
}

func (n *Node) removePeer(id string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	delete(n.peers, id)
	delete(n.links, linkKey(n.id, id))
	for target, route := range n.routes {
		if route.Via == id && target != id {
			delete(n.routes, target)
		}
	}
	if route, ok := n.routes[id]; ok {
		route.Direct = false
		route.UpdatedAt = time.Now()
		n.routes[id] = route
	}
}

func (n *Node) learnPeers(peers []protocol.PeerInfo, via string) {
	changed := false
	for _, peer := range peers {
		if peer.NodeID == "" || peer.NodeID == n.id {
			continue
		}
		if peer.Via != "" && peer.Direct {
			n.learnLink(peer.Via, peer.NodeID, "direct", "learned")
		}
		hops := peer.TTL + 1
		if hops <= 1 {
			hops = 1
		}
		if n.learnRoute(peer.NodeID, peer.Addr, via, hops, false) {
			changed = true
		}
	}
	if changed {
		n.broadcastPeers()
	}
}

func (n *Node) learnRoute(id, addr, via string, hops int, direct bool) bool {
	if id == "" || id == n.id || via == n.id {
		return false
	}
	if hops <= 0 {
		hops = 1
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if addr != "" {
		n.known[id] = addr
	}
	current, ok := n.routes[id]
	if ok && current.Direct && !direct {
		return false
	}
	if ok && current.Hops <= hops && current.Via != via && !direct {
		return false
	}
	if addr == "" {
		addr = current.Addr
	}
	n.routes[id] = Route{NodeID: id, Addr: addr, Via: via, Hops: hops, Direct: direct, UpdatedAt: time.Now()}
	n.notifyKnownLocked()
	return true
}

func (n *Node) learnLink(from, to, kind, health string) {
	if from == "" || to == "" || from == to {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	n.links[linkKey(from, to)] = TopologyLink{From: from, To: to, Kind: kind, Health: health}
}

func linkKey(a, b string) string {
	if a < b {
		return a + "->" + b
	}
	return b + "->" + a
}

func (n *Node) StartMaintenance() {
	go n.heartbeatLoop()
	go n.retryLoop()
	go n.routeExpiryLoop()
}

func (n *Node) heartbeatLoop() {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		n.mu.RLock()
		peers := make([]*Peer, 0, len(n.peers))
		for _, peer := range n.peers {
			peers = append(peers, peer)
		}
		n.mu.RUnlock()
		for _, peer := range peers {
			id := randomID()
			peer.markPing()
			peer.send(protocol.TypeHeartbeat, protocol.HeartbeatBody{ID: id, From: n.id, Time: time.Now().UnixMilli()})
			if time.Since(peer.lastSeen()) > 10*time.Second {
				peer.setUnhealthy()
			}
		}
	}
}

func (n *Node) retryLoop() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		var retries []Delivery
		n.mu.Lock()
		for id, delivery := range n.pending {
			if delivery.Status != "pending" || now.Sub(delivery.LastSent) < 4*time.Second {
				continue
			}
			if delivery.Attempts >= 3 {
				delivery.Status = "failed"
				n.pending[id] = delivery
				fmt.Printf("\ndelivery failed for %s to %s\n> ", id, short(delivery.Target))
				continue
			}
			delivery.Attempts++
			delivery.LastSent = now
			n.pending[id] = delivery
			retries = append(retries, delivery)
		}
		n.mu.Unlock()
		for _, delivery := range retries {
			n.tracef("retry SEND id=%s attempt=%d target=%s", delivery.ID, delivery.Attempts, short(delivery.Target))
			n.route(protocol.TypeSend, protocol.SendBody{RouteHeader: newHeader(delivery.ID, n.id, delivery.Target), Payload: delivery.Payload}, "")
		}
	}
}

func (n *Node) routeExpiryLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		cutoff := time.Now().Add(-60 * time.Second)
		changed := false
		n.mu.Lock()
		for id, route := range n.routes {
			if id == n.id || route.Direct {
				continue
			}
			if route.UpdatedAt.Before(cutoff) {
				delete(n.routes, id)
				changed = true
			}
		}
		n.mu.Unlock()
		if changed {
			n.tracef("expired stale routes")
			n.broadcastPeers()
		}
	}
}

func (n *Node) trackDelivery(id, target, payload string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.pending[id] = Delivery{ID: id, Target: target, Payload: payload, Status: "pending", Attempts: 1, LastSent: time.Now()}
}

func (n *Node) markDelivered(id, from string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	delivery, ok := n.pending[id]
	if !ok {
		return
	}
	delivery.Status = "delivered"
	delivery.Target = from
	n.pending[id] = delivery
}

func (n *Node) notifyKnownLocked() {
	if n.onKnown == nil {
		return
	}
	known := make(map[string]string, len(n.known))
	for id, addr := range n.known {
		known[id] = addr
	}
	go n.onKnown(known)
}

func (n *Node) broadcastPeers() {
	n.mu.RLock()
	peers := make([]protocol.PeerInfo, 0, len(n.routes))
	for id, route := range n.routes {
		if id == n.id {
			continue
		}
		peers = append(peers, protocol.PeerInfo{NodeID: id, Addr: route.Addr, TTL: route.Hops, Via: route.Via, Direct: route.Direct})
	}
	peers = append(peers, protocol.PeerInfo{NodeID: n.id, Addr: n.addr, TTL: 0})
	for id, peer := range n.peers {
		peers = append(peers, protocol.PeerInfo{NodeID: id, Addr: peer.Addr, TTL: 1, Via: n.id, Direct: true})
	}
	n.mu.RUnlock()
	n.broadcast(protocol.TypePeers, protocol.PeerListBody{Peers: peers}, "")
}

func (n *Node) route(frameType string, body any, except string) {
	raw, err := json.Marshal(body)
	if err != nil {
		return
	}
	target, ttl := routeMeta(body)
	if ttl <= 0 {
		n.tracef("drop %s target=%s reason=ttl-expired", frameType, short(target))
		return
	}
	raw = n.decrementTTL(body)
	n.routeRaw(frameType, target, raw, except)
}

func (n *Node) decrementTTL(body any) []byte {
	switch b := body.(type) {
	case protocol.PingBody:
		b.TTL--
		raw, _ := json.Marshal(b)
		return raw
	case protocol.PongBody:
		b.TTL--
		raw, _ := json.Marshal(b)
		return raw
	case protocol.SendBody:
		b.TTL--
		raw, _ := json.Marshal(b)
		return raw
	case protocol.AckBody:
		b.TTL--
		raw, _ := json.Marshal(b)
		return raw
	case protocol.ObjectBody:
		b.TTL--
		raw, _ := json.Marshal(b)
		return raw
	default:
		raw, _ := json.Marshal(body)
		return raw
	}
}

func routeMeta(body any) (string, int) {
	switch b := body.(type) {
	case protocol.PingBody:
		return b.Target, b.TTL
	case protocol.PongBody:
		return b.Target, b.TTL
	case protocol.SendBody:
		return b.Target, b.TTL
	case protocol.AckBody:
		return b.Target, b.TTL
	case protocol.ObjectBody:
		return b.Target, b.TTL
	default:
		return "", protocol.DefaultTTL
	}
}

func (n *Node) routeRaw(frameType, target string, raw json.RawMessage, except string) {
	if target != "" {
		n.mu.RLock()
		route := n.routes[target]
		direct := n.peers[target]
		next := n.peers[route.Via]
		n.mu.RUnlock()
		if direct != nil {
			n.tracef("%s -> %s direct", frameType, short(target))
			direct.sendRaw(frameType, raw)
			return
		}
		if next != nil {
			n.tracef("%s -> %s via %s", frameType, short(target), short(route.Via))
			next.sendRaw(frameType, raw)
			return
		}
	}
	n.tracef("%s -> %s flood", frameType, short(target))
	n.broadcastRaw(frameType, raw, except)
}

func (n *Node) broadcast(frameType string, body any, except string) {
	frame, err := protocol.EncodeFrame(frameType, body)
	if err != nil {
		return
	}
	n.broadcastFrame(frame, except)
}

func (n *Node) broadcastRaw(frameType string, raw json.RawMessage, except string) {
	n.broadcastFrame(protocol.Frame{Version: protocol.Version, Type: frameType, Body: raw}, except)
}

func (n *Node) broadcastFrame(frame protocol.Frame, except string) {
	n.mu.RLock()
	peers := make([]*Peer, 0, len(n.peers))
	for id, peer := range n.peers {
		if id != except {
			peers = append(peers, peer)
		}
	}
	n.mu.RUnlock()
	for _, peer := range peers {
		peer.sendFrame(frame)
	}
}

func (n *Node) markSeen(id string) bool {
	if id == "" {
		return false
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.seen[id] {
		return true
	}
	n.seen[id] = true
	return false
}

func (n *Node) objectByID(id string) (protocol.ObjectBody, bool) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	obj, ok := n.objects[id]
	return obj, ok
}

func (n *Node) tracef(format string, args ...any) {
	n.mu.RLock()
	enabled := n.trace
	n.mu.RUnlock()
	if enabled {
		fmt.Printf("\ntrace: "+format+"\n> ", args...)
	}
}

func (n *Node) monitorSend(body protocol.SendBody) {
	n.mu.RLock()
	enabled := n.monitor
	n.mu.RUnlock()
	if enabled {
		fmt.Printf("\nmonitor: SEND %s -> %s: %s\n> ", short(body.From), short(body.Target), body.Payload)
	}
}

func (p *Peer) send(frameType string, body any) {
	frame, err := protocol.EncodeFrame(frameType, body)
	if err != nil {
		return
	}
	p.sendFrame(frame)
}

func (p *Peer) sendRaw(frameType string, raw json.RawMessage) {
	p.sendFrame(protocol.Frame{Version: protocol.Version, Type: frameType, Body: raw})
}

func (p *Peer) sendFrame(frame protocol.Frame) {
	p.mu.Lock()
	defer p.mu.Unlock()
	_ = p.enc.Encode(frame)
}

func (p *Peer) touch() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.LastSeen = time.Now()
}

func (p *Peer) markPing() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.LastPing = time.Now()
}

func (p *Peer) setHealthy(latency time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.LastSeen = time.Now()
	p.Latency = latency
	p.Healthy = true
}

func (p *Peer) setUnhealthy() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Healthy = false
}

func (p *Peer) lastSeen() time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.LastSeen
}

func (p *Peer) isHealthy() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.Healthy
}

func writeFrame(enc *json.Encoder, frameType string, body any) error {
	frame, err := protocol.EncodeFrame(frameType, body)
	if err != nil {
		return err
	}
	return enc.Encode(frame)
}

func (n *Node) snapshotRoutes() []Route {
	n.mu.RLock()
	defer n.mu.RUnlock()
	routes := make([]Route, 0, len(n.routes))
	for _, route := range n.routes {
		routes = append(routes, route)
	}
	sort.Slice(routes, func(i, j int) bool {
		return routes[i].NodeID < routes[j].NodeID
	})
	return routes
}
