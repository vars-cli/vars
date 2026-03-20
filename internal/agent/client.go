package agent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"time"
)

const dialTimeout = 2 * time.Second

// Get retrieves a value from the agent.
func Get(sockPath, key string) (string, error) {
	resp, err := roundTrip(sockPath, &Request{Op: "get", Key: key})
	if err != nil {
		return "", err
	}
	if !resp.OK {
		return "", fmt.Errorf("%s", resp.Error)
	}
	return resp.Value, nil
}

// List retrieves all keys from the agent.
func List(sockPath string) ([]string, error) {
	resp, err := roundTrip(sockPath, &Request{Op: "list"})
	if err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, fmt.Errorf("%s", resp.Error)
	}
	return resp.Keys, nil
}

// Stop signals the agent to wipe memory and exit.
func Stop(sockPath string) error {
	resp, err := roundTrip(sockPath, &Request{Op: "stop"})
	if err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("%s", resp.Error)
	}
	return nil
}

// IsRunning checks if an agent is reachable at the given socket path.
func IsRunning(sockPath string) bool {
	conn, err := net.DialTimeout("unix", sockPath, dialTimeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func roundTrip(sockPath string, req *Request) (*Response, error) {
	conn, err := net.DialTimeout("unix", sockPath, dialTimeout)
	if err != nil {
		return nil, fmt.Errorf("connecting to agent: %w", err)
	}
	defer conn.Close()

	// Set deadline for the entire operation
	conn.SetDeadline(time.Now().Add(5 * time.Second))

	data, err := MarshalRequest(req)
	if err != nil {
		return nil, err
	}
	if _, err := conn.Write(data); err != nil {
		return nil, fmt.Errorf("writing to agent: %w", err)
	}

	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("reading from agent: %w", err)
		}
		return nil, fmt.Errorf("agent closed connection")
	}

	var resp Response
	if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("parsing agent response: %w", err)
	}
	return &resp, nil
}
