package node

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"meshnet/internal/naming"
	"meshnet/internal/protocol"
)

func (n *Node) REPL(in io.Reader) {
	defer func() {
		if recovered := recover(); recovered != nil {
			errf("terminal recovered from an internal UI error: %v", recovered)
		}
	}()
	n.printWelcome()
	scanner := bufio.NewScanner(in)
	for {
		fmt.Print(n.prompt())
		if !scanner.Scan() {
			return
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if !n.handleCommand(line, scanner) {
			return
		}
	}
}

func (n *Node) handleCommand(line string, scanner *bufio.Scanner) bool {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return true
	}
	cmd := strings.ToLower(parts[0])
	switch cmd {
	case "quit", "exit", "q":
		return false
	case "help", "h", "?":
		n.printHelp()
	case "menu", "m":
		n.runMenu(scanner)
	case "id":
		fmt.Printf("%s  %s\n", titleStyle.Render(n.id), mutedStyle.Render(n.addr))
	case "status", "st":
		n.printStatus()
	case "list", "ls":
		n.printSummary()
	case "connect", "c":
		if len(parts) < 2 {
			errf("usage: connect <name_or_addr>  |  c beta")
			return true
		}
		target := naming.ConnectTarget(parts[1])
		if n.isConnectedAddr(target) {
			okf("already connected to %s", target)
			return true
		}
		if err := n.Connect(target); err != nil {
			errf("%v", err)
			return true
		}
		okf("connecting to %s", target)
	case "trace", "tr":
		if len(parts) < 2 {
			n.mu.RLock()
			enabled := n.trace
			n.mu.RUnlock()
			okf("trace: %v", enabled)
			return true
		}
		enabled := parts[1] == "on" || parts[1] == "true" || parts[1] == "1"
		n.SetTrace(enabled)
		okf("trace: %v", enabled)
	case "monitor", "mon":
		if len(parts) < 2 {
			n.mu.RLock()
			enabled := n.monitor
			n.mu.RUnlock()
			okf("monitor: %v", enabled)
			return true
		}
		enabled := parts[1] == "on" || parts[1] == "true" || parts[1] == "1"
		n.SetMonitor(enabled)
		okf("monitor: %v", enabled)
	case "peers", "online", "users", "u":
		n.printPeers(false)
	case "known", "k":
		n.printPeers(true)
	case "routes", "r":
		n.printRoutes()
	case "deliveries", "d":
		n.printDeliveries()
	case "objects", "o":
		n.printObjects()
	case "ping", "p":
		if len(parts) < 2 {
			errf("usage: ping <node>  |  p beta")
			return true
		}
		target, ok := n.resolveNode(parts[1])
		if !ok {
			errf("unknown or ambiguous node id")
			return true
		}
		id := randomID()
		n.markSeen(id)
		okf("sent ping %s to %s", short(id), target)
		n.route(protocol.TypePing, protocol.PingBody{RouteHeader: newHeader(id, n.id, target)}, "")
	case "send", "s":
		if len(parts) < 3 {
			errf("usage: send <node> <text>  |  s beta hello")
			return true
		}
		target, ok := n.resolveNode(parts[1])
		if !ok {
			errf("unknown or ambiguous node id")
			return true
		}
		id := randomID()
		payload := strings.Join(parts[2:], " ")
		n.markSeen(id)
		n.trackDelivery(id, target, payload)
		okf("sent %s to %s", short(id), target)
		n.route(protocol.TypeSend, protocol.SendBody{RouteHeader: newHeader(id, n.id, target), Payload: payload}, "")
	case "put":
		if len(parts) < 3 {
			errf("usage: put <name> <text>")
			return true
		}
		content := strings.Join(parts[2:], " ")
		sum := sha256.Sum256([]byte(parts[1] + "\x00" + content))
		id := hex.EncodeToString(sum[:])
		obj := protocol.ObjectBody{
			RouteHeader: protocol.RouteHeader{ID: id, From: n.id, TTL: protocol.DefaultTTL, Path: []string{n.id}},
			Name:        parts[1],
			Content:     content,
		}
		n.mu.Lock()
		n.objects[id] = obj
		n.mu.Unlock()
		n.markSeen(id)
		n.route(protocol.TypeObjectPut, obj, "")
		okf("stored and announced object %s", short(id))
	case "get", "g":
		if len(parts) < 2 {
			errf("usage: get <object_or_name> [node]")
			return true
		}
		objectID, ok := n.resolveObject(parts[1])
		if !ok {
			objectID = parts[1]
		}
		if obj, found := n.objectByID(objectID); found {
			okf("object %s (%s): %s", short(obj.ID), obj.Name, obj.Content)
			return true
		}
		target := ""
		if len(parts) > 2 {
			var found bool
			target, found = n.resolveNode(parts[2])
			if !found {
				errf("unknown or ambiguous node id")
				return true
			}
		}
		reqID := randomID()
		n.markSeen(reqID)
		okf("requested object %s", objectID)
		n.route(protocol.TypeObjectGet, protocol.ObjectBody{
			RouteHeader: protocol.RouteHeader{ID: objectID, From: n.id, Target: target, TTL: protocol.DefaultTTL, Path: []string{n.id}},
			RequestID:   reqID,
		}, "")
	default:
		errf("unknown command. type h or m")
	}
	return true
}

func (n *Node) printWelcome() {
	body := titleStyle.Render("MeshNet CLI") + "\n" +
		labelf("node", n.id) + mutedStyle.Render("  ") + labelf("addr", n.addr) + "\n" +
		mutedStyle.Render("menu ") + commandStyle.Render("m") +
		mutedStyle.Render("   send ") + commandStyle.Render("s beta hello") +
		mutedStyle.Render("   overview ") + commandStyle.Render("ls")
	fmt.Println(boxStyle.Render(body))
}

func (n *Node) printHelp() {
	fmt.Println(titleStyle.Render("Commands"))
	fmt.Println(commandStyle.Render("m") + " menu")
	fmt.Println(commandStyle.Render("s <node> <text>") + " send message")
	fmt.Println(commandStyle.Render("c <node>") + " connect by name")
	fmt.Println(commandStyle.Render("p <node>") + " ping")
	fmt.Println(commandStyle.Render("u") + " online peers")
	fmt.Println(commandStyle.Render("r") + " routes")
	fmt.Println(commandStyle.Render("d") + " deliveries")
	fmt.Println(commandStyle.Render("o") + " objects")
	fmt.Println(commandStyle.Render("st") + " status")
	fmt.Println(commandStyle.Render("ls") + " overview")
	fmt.Println(commandStyle.Render("put <name> <text>") + " store object")
	fmt.Println(commandStyle.Render("g <object>") + " get object")
	fmt.Println(commandStyle.Render("q") + " quit")
}

func (n *Node) runMenu(scanner *bufio.Scanner) {
	fmt.Println(titleStyle.Render("MeshNet Menu"))
	fmt.Println(commandStyle.Render("1") + " Send message")
	fmt.Println(commandStyle.Render("2") + " Connect node")
	fmt.Println(commandStyle.Render("3") + " Overview")
	fmt.Println(commandStyle.Render("4") + " Online users")
	fmt.Println(commandStyle.Render("5") + " Routes")
	fmt.Println(commandStyle.Render("6") + " Deliveries")
	fmt.Println(commandStyle.Render("7") + " Objects")
	fmt.Println(commandStyle.Render("8") + " Status")
	fmt.Println(commandStyle.Render("9") + " Toggle trace")
	fmt.Println(commandStyle.Render("0") + " Back")

	choice := promptText(scanner, "Choose")
	switch choice {
	case "1":
		target := promptText(scanner, "Target node")
		message := promptText(scanner, "Message")
		if target != "" && message != "" {
			n.executeLine("send " + target + " " + message)
		}
	case "2":
		target := promptText(scanner, "Node name or address")
		if target != "" {
			n.executeLine("connect " + target)
		}
	case "3":
		n.printSummary()
	case "4":
		n.printPeers(false)
	case "5":
		n.printRoutes()
	case "6":
		n.printDeliveries()
	case "7":
		n.printObjects()
	case "8":
		n.printStatus()
	case "9":
		n.mu.RLock()
		enabled := n.trace
		n.mu.RUnlock()
		n.SetTrace(!enabled)
		okf("trace: %v", !enabled)
	case "0", "":
		return
	default:
		warnf("unknown menu choice")
	}
}

func promptText(scanner *bufio.Scanner, label string) string {
	fmt.Print(commandStyle.Render(label) + mutedStyle.Render(": "))
	if scanner == nil || !scanner.Scan() {
		return ""
	}
	return strings.TrimSpace(scanner.Text())
}

func (n *Node) executeLine(line string) {
	n.handleCommand(line, nil)
}

func (n *Node) resolveNode(prefix string) (string, bool) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	if prefix == n.id || strings.HasPrefix(n.id, prefix) {
		return n.id, true
	}
	matches := make([]string, 0, 1)
	for id := range n.known {
		if strings.HasPrefix(id, prefix) {
			matches = append(matches, id)
		}
	}
	for id := range n.routes {
		if strings.HasPrefix(id, prefix) {
			matches = append(matches, id)
		}
	}
	matches = unique(matches)
	return single(matches)
}

func (n *Node) resolveObject(query string) (string, bool) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	matches := make([]string, 0, 1)
	for id, obj := range n.objects {
		if id == query || obj.Name == query || strings.HasPrefix(id, query) {
			matches = append(matches, id)
		}
	}
	matches = unique(matches)
	return single(matches)
}

func single(values []string) (string, bool) {
	if len(values) != 1 {
		return "", false
	}
	return values[0], true
}

func (n *Node) printPeers(includeKnown bool) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	if includeKnown {
		printMap("known nodes", n.known)
		return
	}
	active := map[string]string{}
	for id, peer := range n.peers {
		health := "healthy"
		if !peer.isHealthy() {
			health = "stale"
		}
		active[id] = peer.Addr + "  " + health
	}
	printMap("connected peers", active)
}

func (n *Node) printStatus() {
	n.mu.RLock()
	defer n.mu.RUnlock()
	fmt.Println(boxStyle.Render(
		labelf("node", titleStyle.Render(n.id)) + "\n" +
			labelf("addr", n.addr) + "\n" +
			labelf("trace", fmt.Sprintf("%v", n.trace)) + mutedStyle.Render("  ") +
			labelf("monitor", fmt.Sprintf("%v", n.monitor)) + "\n" +
			labelf("peers", fmt.Sprintf("%d", len(n.peers))) + mutedStyle.Render("  ") +
			labelf("routes", fmt.Sprintf("%d", len(n.routes))) + mutedStyle.Render("  ") +
			labelf("objects", fmt.Sprintf("%d", len(n.objects))) + mutedStyle.Render("  ") +
			labelf("pending", fmt.Sprintf("%d", len(n.pending))),
	))
}

func (n *Node) printSummary() {
	n.printStatus()
	n.printPeers(false)
	n.printRoutes()
}

func (n *Node) printRoutes() {
	routes := n.snapshotRoutes()
	fmt.Println("routes:")
	for _, route := range routes {
		via := route.Via
		if via == "" {
			via = "self"
		}
		age := "fresh"
		if !route.UpdatedAt.IsZero() {
			age = time.Since(route.UpdatedAt).Round(time.Second).String()
		}
		fmt.Printf("  %s  via=%s  hops=%d  age=%s  addr=%s\n", route.NodeID, via, route.Hops, age, route.Addr)
	}
}

func (n *Node) printObjects() {
	n.mu.RLock()
	defer n.mu.RUnlock()
	if len(n.objects) == 0 {
		fmt.Println("objects: none")
		return
	}
	keys := make([]string, 0, len(n.objects))
	for id := range n.objects {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	fmt.Println("objects:")
	for _, id := range keys {
		obj := n.objects[id]
		fmt.Printf("  %s  %s  from %s\n", id, obj.Name, short(obj.From))
	}
}

func (n *Node) printDeliveries() {
	n.mu.RLock()
	defer n.mu.RUnlock()
	if len(n.pending) == 0 {
		fmt.Println("deliveries: none")
		return
	}
	keys := make([]string, 0, len(n.pending))
	for id := range n.pending {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	fmt.Println("deliveries:")
	for _, id := range keys {
		d := n.pending[id]
		fmt.Printf("  %s  target=%s  status=%s  attempts=%d\n", d.ID, d.Target, d.Status, d.Attempts)
	}
}

func printMap(title string, values map[string]string) {
	if len(values) == 0 {
		fmt.Println(title + ": none")
		return
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	fmt.Println(title + ":")
	for _, key := range keys {
		fmt.Printf("  %s  %s\n", key, values[key])
	}
}
