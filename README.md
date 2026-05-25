# MeshNet

MeshNet is a first draft of **MNP**, a tiny peer-to-peer mesh protocol written in Go.

The goal is not to build "just chat". Chat is only one test behavior. The real project is a protocol where nodes can:

- identify themselves with a node ID
- connect to peers over TCP
- exchange protocol frames
- discover known nodes
- route messages through connected peers
- learn next-hop routes instead of always flooding
- stop runaway forwarding with TTL hop limits
- announce and fetch small named objects

## Project Structure

```txt
cmd/meshnet/          CLI entrypoint
internal/protocol/   MNP wire frames and message bodies
internal/node/       peer connections, routing table, REPL, object store
```

## Protocol Frame

Every wire message is a JSON frame:

```json
{
  "version": "MNP/1",
  "type": "HELLO",
  "body": {
    "node_id": "abc123",
    "addr": "127.0.0.1:4001"
  }
}
```

Current frame types:

- `HELLO`
- `PEERS`
- `PING`
- `PONG`
- `SEND`
- `ACK`
- `OBJECT_PUT`
- `OBJECT_GET`
- `OBJECT_FOUND`
- `HEARTBEAT`
- `HEARTBEAT_OK`

## Run

Open three terminals from this folder.

Fast mode for daily use:

```powershell
go build -o meshnet.exe ./cmd/meshnet
.\meshnet.exe admin
.\meshnet.exe
.\meshnet.exe
```

Or use the helper scripts, which build once only if `meshnet.exe` is missing:

```powershell
.\run-admin.ps1
.\run-node.ps1
.\run-node.ps1
```

Simple mode:

```powershell
go run ./cmd/meshnet admin
go run ./cmd/meshnet alpha
go run ./cmd/meshnet beta
go run ./cmd/meshnet gamma beta
```

You can also let MeshNet choose the normal node names for you:

```powershell
go run ./cmd/meshnet
```

The first free name is selected in order: `alpha`, `beta`, `gamma`, `delta`, and so on.

In simple mode:

- `admin` listens on `127.0.0.1:4000`
- `admin` starts the dashboard on `http://127.0.0.1:8000`
- `admin` monitors routed `SEND` messages
- normal nodes auto-connect to `admin` when no connect names are given
- named nodes use predictable ports: `alpha=4001`, `beta=4002`, `gamma=4003`, `delta=4004`
- unnamed nodes auto-pick the first free friendly name and matching port
- use extra names to connect somewhere specific: `go run ./cmd/meshnet gamma beta`
- flags still work, but put them before the name: `go run ./cmd/meshnet --dashboard 8001 admin`

Open:

```txt
http://127.0.0.1:8000
```

You can still use explicit flags:

```powershell
go run ./cmd/meshnet --port 4001 --id alpha --config alpha.json --dashboard 8001
```

Use separate config files when running multiple local nodes:

```powershell
go run ./cmd/meshnet --port 4001 --id alpha --config alpha.json --dashboard 8000
go run ./cmd/meshnet --port 4002 --id beta --config beta.json --connect 127.0.0.1:4001
```

Inside any node:

```txt
id
status
peers
known
objects
routes
deliveries
trace on
monitor on
connect beta
ping <node_id_or_prefix>
send <node_id_or_prefix> hello from another node
put note hello mesh
get <object_id_or_name>
get <object_id_or_name> <node_id_or_prefix>
```

Short aliases:

```txt
m                  open interactive menu
h                  help
s beta hello       send message
c beta             connect to beta
p beta             ping beta
u                  online users / peers
r                  routes
d                  deliveries
o                  objects
st                 status
ls                 overview
g note             get object
q                  quit
```

The terminal also has a guided menu:

```txt
m
```

Use the arrow keys and Enter to choose actions like sending a message, connecting to a node, viewing routes, or checking deliveries.

Node IDs can be full IDs or unique prefixes. If a node prints `010a8dfc57...`, this works:

```txt
send alpha hello
```

## What Makes This A Protocol Project

This project defines the rules for how nodes speak to each other. Later versions can replace the JSON body with a binary frame, add message TTLs, acknowledgements, retries, encryption, better routing tables, and a JavaScript dashboard that visualizes live topology.

The current protocol already includes TTL, route learning, message acknowledgements, retries, stale route expiry, heartbeat health checks, persistent config, and traceable next-hop decisions.

## Advanced Protocol Features

Current advanced layer:

- persistent node ID and known peer storage in `meshnet.json`
- heartbeat frames: `HEARTBEAT` and `HEARTBEAT_OK`
- peer health and latency tracking
- ACK-based delivery status for `SEND`
- retry loop for pending messages
- route timestamps and stale route expiry
- dashboard visibility for peer health and deliveries

## Dashboard

The dashboard is served by any node when you pass `--dashboard <port>`.

It shows:

- node ID and address
- connected peers with health and latency
- online users
- known nodes
- learned routes and next hops
- live topology links between connected nodes
- delivery status
- stored objects
- a small live topology map

The browser polls `/api/state` once per second.
