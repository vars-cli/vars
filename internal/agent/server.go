package agent

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"os/signal"
	"sort"
	"sync"
	"syscall"
	"time"
)

// Server holds decrypted store data in memory and serves it over a Unix socket.
type Server struct {
	data     map[string]string
	mu       sync.RWMutex
	listener net.Listener
	sockPath string
	done     chan struct{}
}

// NewServer creates a new agent server with the given data.
func NewServer(data map[string]string, sockPath string) *Server {
	return &Server{
		data:     data,
		sockPath: sockPath,
		done:     make(chan struct{}),
	}
}

// Start listens on the Unix socket and serves requests.
// ttl of 0 means no expiry.
// This blocks until Stop is called or the TTL expires.
func (s *Server) Start(ttl time.Duration) error {
	// Remove stale socket
	os.Remove(s.sockPath)

	var err error
	s.listener, err = net.Listen("unix", s.sockPath)
	if err != nil {
		return err
	}

	// Set socket permissions
	os.Chmod(s.sockPath, 0600)

	// TTL timer
	if ttl > 0 {
		time.AfterFunc(ttl, func() {
			s.Stop()
		})
	}

	// Signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		select {
		case <-sigCh:
			s.Stop()
		case <-s.done:
		}
	}()

	// Accept loop
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.done:
				return nil // clean shutdown
			default:
				continue
			}
		}
		go s.handleConn(conn)
	}
}

// Stop wipes memory, closes the socket, and cleans up.
func (s *Server) Stop() {
	select {
	case <-s.done:
		return // already stopped
	default:
	}
	close(s.done)

	// Wipe memory
	s.mu.Lock()
	for k := range s.data {
		delete(s.data, k)
	}
	s.mu.Unlock()

	if s.listener != nil {
		s.listener.Close()
	}
	os.Remove(s.sockPath)
}

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		var req Request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			resp, _ := MarshalResponse(&Response{OK: false, Error: "invalid request"})
			conn.Write(resp)
			continue
		}

		resp := s.handleRequest(&req)
		data, _ := MarshalResponse(resp)
		conn.Write(data)

		if req.Op == "stop" {
			s.Stop()
			return
		}
	}
}

func (s *Server) handleRequest(req *Request) *Response {
	switch req.Op {
	case "get":
		s.mu.RLock()
		defer s.mu.RUnlock()
		val, ok := s.data[req.Key]
		if !ok {
			return &Response{OK: false, Error: "key not found"}
		}
		return &Response{OK: true, Value: val}

	case "list":
		s.mu.RLock()
		defer s.mu.RUnlock()
		keys := make([]string, 0, len(s.data))
		for k := range s.data {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return &Response{OK: true, Keys: keys}

	case "stop":
		return &Response{OK: true}

	default:
		return &Response{OK: false, Error: "unknown op: " + req.Op}
	}
}
