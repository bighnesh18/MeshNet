package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"meshnet/internal/config"
	"meshnet/internal/naming"
	"meshnet/internal/node"
)

func main() {
	port := flag.String("port", "", "port to listen on")
	host := flag.String("host", "0.0.0.0", "host/interface to listen on")
	advertise := flag.String("advertise", "", "address peers should use to reach this node, like 192.168.1.5:4000")
	connect := flag.String("connect", "", "peer address to connect to, like 127.0.0.1:4001")
	nodeID := flag.String("id", "", "optional stable node id for demos and tests")
	trace := flag.Bool("trace", false, "print routing decisions")
	dashboard := flag.String("dashboard", "", "optional dashboard port, like 8000")
	dashboardHost := flag.String("dashboard-host", "0.0.0.0", "dashboard host/interface")
	configPath := flag.String("config", "", "path to node config file")
	monitor := flag.Bool("monitor", false, "print routed SEND messages that pass through this node")
	flag.Parse()

	args := flag.Args()
	name := *nodeID
	if name == "" && len(args) > 0 {
		name = args[0]
	}
	if name == "" {
		name = naming.FirstAvailableName()
		fmt.Printf("auto-selected node name %s\n", name)
	}
	if *port == "" {
		*port = fmt.Sprintf("%d", naming.Port(name))
	}
	if *configPath == "" {
		*configPath = filepath.Join("configs", name+".json")
	}
	if *dashboard == "" && name == "admin" {
		// On Railway, $PORT is the only port exposed to the public internet for HTTP.
		// Use it if set, otherwise fall back to the default 8000 for local development.
		if envPort := os.Getenv("PORT"); envPort != "" && envPort != *port {
			*dashboard = envPort
		} else {
			*dashboard = "8000"
		}
	}
	// Also respect $PORT for the node TCP listener only if explicitly running as admin
	// and no port flag was given. Railway reserves $PORT for HTTP, so we keep
	// the mesh node on its fixed port (4000) and only use $PORT for the dashboard.
	if name == "admin" {
		*monitor = true
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("config load failed: %v", err)
	}
	id := name
	if id == "" {
		id = cfg.NodeID
	}
	saveConfig := func(known map[string]string) {
		cfg.NodeID = id
		cfg.KnownPeers = known
		if err := config.Save(*configPath, cfg); err != nil {
			log.Printf("config save failed: %v", err)
		}
	}

	listenAddr := *host + ":" + *port
	advertiseAddr := *advertise
	if advertiseAddr == "" {
		advertiseAddr = listenAddr
	}

	n := node.NewWithOptions(advertiseAddr, id, node.Options{
		KnownPeers: cfg.KnownPeers,
		OnKnown:    saveConfig,
		ListenAddr: listenAddr,
	})
	id = n.ID()
	if cfg.NodeID == "" || cfg.NodeID != n.ID() {
		cfg.NodeID = n.ID()
		if cfg.KnownPeers == nil {
			cfg.KnownPeers = map[string]string{}
		}
		if err := config.Save(*configPath, cfg); err != nil {
			log.Printf("config save failed: %v", err)
		}
	}
	n.SetTrace(*trace)
	n.SetMonitor(*monitor)
	n.StartMaintenance()

	go n.Listen()
	if *dashboard != "" {
		go func() {
			addr := *dashboardHost + ":" + *dashboard
			fmt.Printf("dashboard listening on http://%s\n", addr)
			if err := n.ServeDashboard(addr); err != nil {
				log.Printf("dashboard failed: %v", err)
			}
		}()
	}

	connectTargets := connectionTargets(*connect, args, name)
	for _, target := range connectTargets {
		target := target
		go func() {
			time.Sleep(250 * time.Millisecond)
			if err := n.Connect(target); err != nil {
				log.Printf("connect %s failed: %v", target, err)
			}
		}()
	}

	absConfig, _ := filepath.Abs(*configPath)
	fmt.Printf("meshnet node %s listening on %s\n", n.ID(), n.Addr())
	fmt.Printf("config: %s\n", absConfig)
	if *dashboard != "" {
		fmt.Printf("dashboard: http://%s:%s\n", *dashboardHost, *dashboard)
	}
	n.REPL(os.Stdin)
}

func connectionTargets(flagConnect string, args []string, self string) []string {
	targets := []string{}
	if flagConnect != "" {
		for _, part := range strings.Split(flagConnect, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				targets = append(targets, part)
			}
		}
	}
	if len(args) > 1 {
		for _, name := range args[1:] {
			targets = append(targets, naming.ConnectTarget(name))
		}
	}
	if len(targets) == 0 && self != "admin" {
		targets = append(targets, naming.Addr("admin"))
	}
	return targets
}
