package node

import (
	"encoding/json"
	"net/http"
	"os"

	"meshnet/internal/protocol"
)

type DashboardState struct {
	ID         string                `json:"id"`
	Addr       string                `json:"addr"`
	Trace      bool                  `json:"trace"`
	Monitor    bool                  `json:"monitor"`
	Peers      []PeerState           `json:"peers"`
	Online     []OnlineUser          `json:"online"`
	Links      []TopologyLink        `json:"links"`
	Known      map[string]string     `json:"known"`
	Routes     []Route               `json:"routes"`
	Objects    []protocol.ObjectBody `json:"objects"`
	Deliveries []Delivery            `json:"deliveries"`
	Seen       int                   `json:"seen"`
}

type PeerState struct {
	ID       string `json:"id"`
	Addr     string `json:"addr"`
	Healthy  bool   `json:"healthy"`
	Latency  string `json:"latency"`
	LastSeen string `json:"last_seen"`
}

type OnlineUser struct {
	ID      string `json:"id"`
	Addr    string `json:"addr"`
	Role    string `json:"role"`
	Healthy bool   `json:"healthy"`
}

type TopologyLink struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Kind   string `json:"kind"`
	Health string `json:"health"`
}

func (n *Node) ServeDashboard(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", dashboardPage)
	mux.HandleFunc("/api/state", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(n.DashboardState())
	})
	mux.HandleFunc("/download", func(w http.ResponseWriter, r *http.Request) {
		exePath, err := os.Executable()
		if err != nil {
			http.Error(w, "Executable not found", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Disposition", "attachment; filename=meshnet.exe")
		http.ServeFile(w, r, exePath)
	})
	return http.ListenAndServe(addr, mux)
}

func (n *Node) DashboardState() DashboardState {
	n.mu.RLock()
	defer n.mu.RUnlock()

	peers := make([]PeerState, 0, len(n.peers))
	online := []OnlineUser{{ID: n.id, Addr: n.addr, Role: "self", Healthy: true}}
	linkSet := map[string]TopologyLink{}
	for _, link := range n.links {
		linkSet[linkKey(link.From, link.To)] = link
	}
	for id, peer := range n.peers {
		peer.mu.Lock()
		healthy := peer.Healthy
		health := "healthy"
		if !healthy {
			health = "stale"
		}
		peers = append(peers, PeerState{
			ID:       id,
			Addr:     peer.Addr,
			Healthy:  healthy,
			Latency:  peer.Latency.String(),
			LastSeen: peer.LastSeen.Format("15:04:05"),
		})
		online = append(online, OnlineUser{ID: id, Addr: peer.Addr, Role: "peer", Healthy: healthy})
		linkSet[linkKey(n.id, id)] = TopologyLink{From: n.id, To: id, Kind: "direct", Health: health}
		peer.mu.Unlock()
	}

	known := make(map[string]string, len(n.known))
	for id, addr := range n.known {
		known[id] = addr
	}

	routes := make([]Route, 0, len(n.routes))
	for _, route := range n.routes {
		routes = append(routes, route)
		if route.NodeID != n.id && !route.Direct && route.Via != "" {
			key := linkKey(route.Via, route.NodeID)
			if _, exists := linkSet[key]; !exists {
				linkSet[key] = TopologyLink{From: route.Via, To: route.NodeID, Kind: "route", Health: "learned"}
			}
		}
	}
	links := make([]TopologyLink, 0, len(linkSet))
	for _, link := range linkSet {
		links = append(links, link)
	}

	objects := make([]protocol.ObjectBody, 0, len(n.objects))
	for _, obj := range n.objects {
		objects = append(objects, obj)
	}

	deliveries := make([]Delivery, 0, len(n.pending))
	for _, delivery := range n.pending {
		deliveries = append(deliveries, delivery)
	}

	return DashboardState{
		ID:         n.id,
		Addr:       n.addr,
		Trace:      n.trace,
		Monitor:    n.monitor,
		Peers:      peers,
		Online:     online,
		Links:      links,
		Known:      known,
		Routes:     routes,
		Objects:    objects,
		Deliveries: deliveries,
		Seen:       len(n.seen),
	}
}

func dashboardPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(dashboardHTML))
}

const dashboardHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>MeshNet Dashboard</title>
  <style>
    :root {
      color-scheme: dark;
      --bg: #101214;
      --panel: #171b1f;
      --panel-2: #1e2429;
      --text: #edf2f4;
      --muted: #9aa7b2;
      --line: #2b343b;
      --accent: #67d4b4;
      --warn: #f2c572;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      font: 14px/1.45 system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      background: var(--bg);
      color: var(--text);
    }
    header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 16px;
      padding: 18px 22px;
      border-bottom: 1px solid var(--line);
      background: #121619;
      position: sticky;
      top: 0;
      z-index: 2;
    }
    h1 { margin: 0; font-size: 18px; font-weight: 700; }
    main {
      display: grid;
      grid-template-columns: minmax(260px, 360px) 1fr;
      gap: 18px;
      padding: 18px;
    }
    section {
      background: var(--panel);
      border: 1px solid var(--line);
      border-radius: 8px;
      min-width: 0;
    }
    section h2 {
      margin: 0;
      padding: 14px 16px;
      font-size: 13px;
      color: var(--muted);
      border-bottom: 1px solid var(--line);
      text-transform: uppercase;
      letter-spacing: 0;
    }
    .grid {
      display: grid;
      grid-template-columns: repeat(2, minmax(0, 1fr));
      gap: 18px;
    }
    .content { padding: 14px 16px; }
    .stat {
      display: grid;
      gap: 3px;
      padding: 12px;
      background: var(--panel-2);
      border: 1px solid var(--line);
      border-radius: 6px;
      margin-bottom: 10px;
    }
    .label { color: var(--muted); font-size: 12px; }
    .value {
      font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
      overflow-wrap: anywhere;
    }
    table { width: 100%; border-collapse: collapse; }
    th, td {
      text-align: left;
      padding: 10px 12px;
      border-bottom: 1px solid var(--line);
      vertical-align: top;
    }
    th { color: var(--muted); font-size: 12px; font-weight: 600; }
    td { font-family: ui-monospace, SFMono-Regular, Consolas, monospace; overflow-wrap: anywhere; }
    .pill {
      display: inline-flex;
      align-items: center;
      min-height: 24px;
      padding: 2px 8px;
      border-radius: 999px;
      background: rgba(103, 212, 180, .13);
      color: var(--accent);
      border: 1px solid rgba(103, 212, 180, .3);
      font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
    }
    .empty { color: var(--muted); padding: 14px 16px; }
    .map {
      min-height: 420px;
      position: relative;
      overflow: hidden;
      background:
        linear-gradient(rgba(43, 52, 59, .45) 1px, transparent 1px),
        linear-gradient(90deg, rgba(43, 52, 59, .45) 1px, transparent 1px);
      background-size: 42px 42px;
    }
    .node {
      position: absolute;
      width: 128px;
      min-height: 62px;
      transform: translate(-50%, -50%);
      display: grid;
      place-items: center;
      text-align: center;
      padding: 8px;
      border: 1px solid var(--line);
      border-radius: 8px;
      background: var(--panel-2);
      font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
      overflow-wrap: anywhere;
      z-index: 1;
    }
    .node.self { border-color: var(--accent); color: var(--accent); box-shadow: 0 0 0 3px rgba(103, 212, 180, .12); }
    .node .sub { color: var(--muted); font-size: 11px; margin-top: 4px; }
    svg { position: absolute; inset: 0; width: 100%; height: 100%; }
    line { stroke: rgba(103, 212, 180, .55); stroke-width: 2.5; }
    line.direct { stroke: rgba(103, 212, 180, .9); }
    line.route { stroke: rgba(242, 197, 114, .75); stroke-dasharray: 5 5; }
    line.stale { stroke: rgba(154, 167, 178, .45); stroke-dasharray: 3 6; }
    @media (max-width: 900px) {
      main, .grid { grid-template-columns: 1fr; }
    }
  </style>
</head>
<body>
  <header>
    <h1>MeshNet Dashboard</h1>
    <div style="display: flex; gap: 12px; align-items: center;">
      <a href="/download" style="color: var(--accent); text-decoration: none; border: 1px solid var(--accent); padding: 4px 12px; border-radius: 4px; font-size: 12px;">Download Node</a>
      <span id="status" class="pill">connecting</span>
    </div>
  </header>
  <main>
    <section>
      <h2>Node</h2>
      <div class="content">
        <div class="stat"><span class="label">ID</span><span id="node-id" class="value"></span></div>
        <div class="stat"><span class="label">Address</span><span id="node-addr" class="value"></span></div>
        <div class="stat"><span class="label">Trace</span><span id="trace" class="value"></span></div>
        <div class="stat"><span class="label">Monitor</span><span id="monitor" class="value"></span></div>
        <div class="stat"><span class="label">Seen Messages</span><span id="seen" class="value"></span></div>
      </div>
    </section>
    <section>
      <h2>Topology</h2>
      <div id="map" class="map"></div>
    </section>
    <div class="grid" style="grid-column: 1 / -1;">
      <section>
        <h2>Routes</h2>
        <div id="routes"></div>
      </section>
      <section>
        <h2>Peers</h2>
        <div id="peers"></div>
      </section>
      <section>
        <h2>Online Users</h2>
        <div id="online"></div>
      </section>
      <section>
        <h2>Known Nodes</h2>
        <div id="known"></div>
      </section>
      <section>
        <h2>Objects</h2>
        <div id="objects"></div>
      </section>
    </div>
  </main>
  <script>
    const short = id => id && id.length > 10 ? id.slice(0, 10) : (id || "");
    const esc = value => String(value ?? "").replace(/[&<>"']/g, ch => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", "\"": "&quot;", "'": "&#39;" }[ch]));

    function table(headers, rows) {
      if (!rows.length) return '<div class="empty">none</div>';
      return '<table><thead><tr>' + headers.map(h => '<th>' + esc(h) + '</th>').join('') + '</tr></thead><tbody>' +
        rows.map(row => '<tr>' + row.map(cell => '<td>' + esc(cell) + '</td>').join('') + '</tr>').join('') +
        '</tbody></table>';
    }

    function drawMap(state) {
      const ids = Array.from(new Set([state.id, ...state.online.map(u => u.id), ...state.routes.map(r => r.NodeID)])).filter(Boolean);
      const centerX = 50, centerY = 50, radius = 34;
      const positions = {};
      positions[state.id] = [centerX, centerY];
      const others = ids.filter(id => id !== state.id);
      others.forEach((id, index) => {
        const angle = (Math.PI * 2 * index / Math.max(others.length, 1)) - Math.PI / 2;
        positions[id] = [centerX + Math.cos(angle) * radius, centerY + Math.sin(angle) * radius];
      });
      const lines = state.links
        .filter(link => positions[link.from] && positions[link.to] && link.from !== link.to)
        .map(link => {
          const a = positions[link.from], b = positions[link.to];
          const cls = link.kind === 'direct' ? (link.health === 'healthy' ? 'direct' : 'stale') : 'route';
          return '<line class="' + cls + '" x1="' + a[0] + '" y1="' + a[1] + '" x2="' + b[0] + '" y2="' + b[1] + '"></line>';
        }).join('');
      const online = new Set(state.online.map(u => u.id));
      const nodes = ids.map(id => {
        const p = positions[id];
        const cls = id === state.id ? 'node self' : 'node';
        const sub = id === state.id ? 'self' : (online.has(id) ? 'online' : 'known');
        return '<div class="' + cls + '" style="left:' + p[0] + '%;top:' + p[1] + '%"><div>' + esc(short(id)) + '</div><div class="sub">' + sub + '</div></div>';
      }).join('');
      document.getElementById('map').innerHTML = '<svg viewBox="0 0 100 100" preserveAspectRatio="none">' + lines + '</svg>' + nodes;
    }

    async function refresh() {
      try {
        const res = await fetch('/api/state', { cache: 'no-store' });
        const state = await res.json();
        document.getElementById('status').textContent = 'live';
        document.getElementById('node-id').textContent = state.id;
        document.getElementById('node-addr').textContent = state.addr;
        document.getElementById('trace').textContent = state.trace;
        document.getElementById('monitor').textContent = state.monitor;
        document.getElementById('seen').textContent = state.seen;
        document.getElementById('routes').innerHTML = table(['node', 'next hop', 'hops', 'addr'], state.routes.map(r => [r.NodeID, r.Direct ? 'direct' : (r.Via || 'self'), r.Hops, r.Addr]));
        document.getElementById('peers').innerHTML = table(['node', 'addr', 'health', 'latency'], state.peers.map(p => [p.id, p.addr, p.healthy ? 'healthy' : 'stale', p.latency]));
        document.getElementById('online').innerHTML = table(['user', 'role', 'addr', 'state'], state.online.map(u => [u.id, u.role, u.addr, u.healthy ? 'online' : 'stale']));
        document.getElementById('known').innerHTML = table(['node', 'addr'], Object.entries(state.known));
        document.getElementById('objects').innerHTML = table(['id', 'name', 'from'], state.objects.map(o => [o.id, o.name, o.from]));
        if (!document.getElementById('deliveries')) {
          const section = document.createElement('section');
          section.innerHTML = '<h2>Deliveries</h2><div id="deliveries"></div>';
          document.querySelector('.grid').appendChild(section);
        }
        document.getElementById('deliveries').innerHTML = table(['id', 'target', 'status', 'attempts'], state.deliveries.map(d => [d.ID, d.Target, d.Status, d.Attempts]));
        drawMap(state);
      } catch (err) {
        document.getElementById('status').textContent = 'offline';
      }
    }
    refresh();
    setInterval(refresh, 1000);
  </script>
</body>
</html>`
