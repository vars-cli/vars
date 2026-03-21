package agent

import (
	"bufio"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sort"
	"sync"
	"syscall"
	"time"

	"github.com/brickpop/secrets/internal/crypto"
	agebackend "github.com/brickpop/secrets/internal/crypto/age"
	"github.com/brickpop/secrets/internal/store"
)

// Server holds decrypted store data in memory and serves it over a Unix socket.
type Server struct {
	data       map[string]string
	dataMu     sync.RWMutex
	passphrase string
	backend    crypto.Backend
	storeDir   string
	sockPath   string
	done       chan struct{}
	ready      chan struct{}
}

// NewServer creates a new agent server with the given data and encryption context.
func NewServer(data map[string]string, sockPath string, passphrase string, backend crypto.Backend, storeDir string) *Server {
	return &Server{
		data:       data,
		sockPath:   sockPath,
		passphrase: passphrase,
		backend:    backend,
		storeDir:   storeDir,
		done:       make(chan struct{}),
		ready:      make(chan struct{}),
	}
}

// Start listens on the Unix socket and serves requests.
// ttl of 0 means no expiry. Blocks until Stop or TTL expiry.
func (s *Server) Start(ttl time.Duration) error {
	os.Remove(s.sockPath)

	ln, err := net.Listen("unix", s.sockPath)
	if err != nil {
		return err
	}

	close(s.ready)

	os.Chmod(s.sockPath, 0600)

	if ttl > 0 {
		time.AfterFunc(ttl, func() {
			s.Stop()
		})
	}

	// Close listener when done fires (from Stop, TTL, or signal).
	go func() {
		<-s.done
		ln.Close()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		select {
		case <-sigCh:
			s.Stop()
		case <-s.done:
		}
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-s.done:
				return nil
			default:
				continue
			}
		}
		go s.handleConn(conn)
	}
}

// Ready returns a channel that is closed when the server is listening.
func (s *Server) Ready() <-chan struct{} {
	return s.ready
}

// Stop wipes memory, closes the socket, and cleans up.
func (s *Server) Stop() {
	select {
	case <-s.done:
		return
	default:
	}
	close(s.done)

	s.dataMu.Lock()
	for k := range s.data {
		delete(s.data, k)
	}
	s.dataMu.Unlock()

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
		s.dataMu.RLock()
		defer s.dataMu.RUnlock()
		val, ok := s.data[req.Key]
		if !ok {
			return &Response{OK: false, Error: "key not found"}
		}
		return &Response{OK: true, Value: val}

	case "list":
		s.dataMu.RLock()
		defer s.dataMu.RUnlock()
		keys := make([]string, 0, len(s.data))
		for k := range s.data {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return &Response{OK: true, Keys: keys}

	case "set":
		return s.handleSet(req)

	case "delete":
		return s.handleDelete(req)

	case "passwd":
		return s.handlePasswd(req)

	case "stop":
		return &Response{OK: true}

	default:
		return &Response{OK: false, Error: "unknown op: " + req.Op}
	}
}

func (s *Server) handleSet(req *Request) *Response {
	s.dataMu.Lock()
	defer s.dataMu.Unlock()

	// Overwriting an existing key requires the passphrase
	if _, exists := s.data[req.Key]; exists {
		if !s.checkPassphrase(req.Passphrase) {
			return &Response{OK: false, Error: ErrPassphraseRequired}
		}
	}

	s.data[req.Key] = req.Value

	if err := store.SaveData(s.data, s.backend, s.storeDir); err != nil {
		// Rollback: remove the key we just set (best effort)
		delete(s.data, req.Key)
		return &Response{OK: false, Error: fmt.Sprintf("saving store: %v", err)}
	}

	return &Response{OK: true}
}

func (s *Server) handleDelete(req *Request) *Response {
	s.dataMu.Lock()
	defer s.dataMu.Unlock()

	if !s.checkPassphrase(req.Passphrase) {
		return &Response{OK: false, Error: ErrPassphraseRequired}
	}

	if _, exists := s.data[req.Key]; !exists {
		return &Response{OK: false, Error: "key not found"}
	}

	delete(s.data, req.Key)

	if err := store.SaveData(s.data, s.backend, s.storeDir); err != nil {
		return &Response{OK: false, Error: fmt.Sprintf("saving store: %v", err)}
	}

	return &Response{OK: true}
}

func (s *Server) handlePasswd(req *Request) *Response {
	s.dataMu.Lock()
	defer s.dataMu.Unlock()

	if !s.checkPassphrase(req.Passphrase) {
		return &Response{OK: false, Error: ErrPassphraseRequired}
	}

	newBackend := agebackend.New(req.NewPassphrase)

	if err := store.SaveData(s.data, newBackend, s.storeDir); err != nil {
		return &Response{OK: false, Error: fmt.Sprintf("saving store: %v", err)}
	}

	// Update internal state
	s.passphrase = req.NewPassphrase
	s.backend = newBackend

	return &Response{OK: true}
}

func (s *Server) checkPassphrase(provided string) bool {
	return subtle.ConstantTimeCompare([]byte(s.passphrase), []byte(provided)) == 1
}
