package protocol

import "encoding/json"

const (
	Version    = "MNP/1"
	DefaultTTL = 8
)

const (
	TypeHello       = "HELLO"
	TypePeers       = "PEERS"
	TypePing        = "PING"
	TypePong        = "PONG"
	TypeSend        = "SEND"
	TypeAck         = "ACK"
	TypeObjectPut   = "OBJECT_PUT"
	TypeObjectGet   = "OBJECT_GET"
	TypeObjectFound = "OBJECT_FOUND"
	TypeHeartbeat   = "HEARTBEAT"
	TypeHeartbeatOK = "HEARTBEAT_OK"
)

type Frame struct {
	Version string          `json:"version"`
	Type    string          `json:"type"`
	Body    json.RawMessage `json:"body"`
}

type RouteHeader struct {
	ID     string   `json:"id"`
	From   string   `json:"from"`
	Target string   `json:"target"`
	TTL    int      `json:"ttl"`
	Path   []string `json:"path,omitempty"`
}

type HelloBody struct {
	NodeID string `json:"node_id"`
	Addr   string `json:"addr"`
}

type PeerInfo struct {
	NodeID string `json:"node_id"`
	Addr   string `json:"addr"`
	TTL    int    `json:"ttl,omitempty"`
	Via    string `json:"via,omitempty"`
	Direct bool   `json:"direct,omitempty"`
}

type PeerListBody struct {
	Peers []PeerInfo `json:"peers"`
}

type PingBody struct {
	RouteHeader
}

type PongBody struct {
	RouteHeader
}

type SendBody struct {
	RouteHeader
	Payload string `json:"payload"`
}

type AckBody struct {
	RouteHeader
}

type HeartbeatBody struct {
	ID   string `json:"id"`
	From string `json:"from"`
	Time int64  `json:"time"`
}

type ObjectBody struct {
	RouteHeader
	RequestID string `json:"request_id,omitempty"`
	Name      string `json:"name"`
	Content   string `json:"content,omitempty"`
}

func EncodeFrame(frameType string, body any) (Frame, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return Frame{}, err
	}
	return Frame{Version: Version, Type: frameType, Body: raw}, nil
}

func DecodeBody(raw json.RawMessage, out any) error {
	return json.Unmarshal(raw, out)
}
