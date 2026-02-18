package daemon

import "encoding/json"

// Request is the JSON message sent from CLI to daemon.
type Request struct {
	Action string          `json:"action"`
	Params json.RawMessage `json:"params,omitempty"`
}

// Response is the JSON message sent from daemon to CLI.
type Response struct {
	OK    bool            `json:"ok"`
	Error string          `json:"error,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
}
