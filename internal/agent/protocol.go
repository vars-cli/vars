// Package agent implements the background agent that holds decrypted
// store data in memory and serves it over a Unix socket.
package agent

import "encoding/json"

// Request is a JSON message from client to agent.
type Request struct {
	Op            string `json:"op"`
	Key           string `json:"key,omitempty"`
	Value         string `json:"value,omitempty"`
	Passphrase    string `json:"passphrase,omitempty"`
	NewPassphrase string `json:"new_passphrase,omitempty"`
}

// Response is a JSON message from agent to client.
type Response struct {
	OK    bool     `json:"ok"`
	Value string   `json:"value,omitempty"`
	Keys  []string `json:"keys,omitempty"`
	Error string   `json:"error,omitempty"`
}

// Well-known error strings used for client-side retry logic.
const ErrPassphraseRequired = "passphrase required"

// MarshalRequest encodes a request as a newline-terminated JSON line.
func MarshalRequest(r *Request) ([]byte, error) {
	data, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// MarshalResponse encodes a response as a newline-terminated JSON line.
func MarshalResponse(r *Response) ([]byte, error) {
	data, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
