package agent

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"crypto/subtle"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/vars-cli/vars/internal/crypto"
	"github.com/vars-cli/vars/internal/store"
)

// Server holds decrypted store data in memory and serves it over a Unix socket via gRPC.
type Server struct {
	UnimplementedVarsServer

	data       map[string]string
	dataMu     sync.RWMutex
	passphrase string
	backend    crypto.Backend
	newBackend func(passphrase string) crypto.Backend
	storeDir   string
	sockPath   string

	timer   *time.Timer
	timerMu sync.Mutex

	grpcSrv *grpc.Server
	done    chan struct{}
	ready   chan struct{}
}

// NewServer creates a new agent server.
// newBackend is a factory used by Passwd to create a replacement backend.
func NewServer(data map[string]string, sockPath, passphrase string, backend crypto.Backend, newBackend func(string) crypto.Backend, storeDir string) *Server {
	return &Server{
		data:       data,
		sockPath:   sockPath,
		passphrase: passphrase,
		backend:    backend,
		newBackend: newBackend,
		storeDir:   storeDir,
		done:       make(chan struct{}),
		ready:      make(chan struct{}),
	}
}

// Start listens on the Unix socket and serves requests.
// ttl of 0 means no expiry. Blocks until Stop or TTL expiry.
func (s *Server) Start(ttl time.Duration) error {
	os.Remove(s.sockPath)

	lis, err := net.Listen("unix", s.sockPath)
	if err != nil {
		return err
	}
	os.Chmod(s.sockPath, 0600)

	// Hook the server to the GRPC request handlers
	s.grpcSrv = grpc.NewServer()
	RegisterVarsServer(s.grpcSrv, s)

	close(s.ready)

	if ttl > 0 {
		s.resetTimer(ttl)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		select {
		case <-sigCh:
			s.Stop()
		case <-s.done:
		}
	}()

	if err := s.grpcSrv.Serve(lis); err != nil {
		select {
		case <-s.done:
			return nil // graceful shutdown
		default:
			return err
		}
	}
	return nil
}

// Ready returns a channel closed when the server is listening.
func (s *Server) Ready() <-chan struct{} {
	return s.ready
}

// Stop drains in-flight RPCs, wipes memory, and removes the socket.
func (s *Server) Stop() {
	select {
	case <-s.done:
		return
	default:
	}
	close(s.done)

	s.timerMu.Lock()
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	s.timerMu.Unlock()

	if s.grpcSrv != nil {
		s.grpcSrv.GracefulStop()
	}

	s.dataMu.Lock()
	for k := range s.data {
		delete(s.data, k)
	}
	s.dataMu.Unlock()

	os.Remove(s.sockPath)
}

func (s *Server) resetTimer(d time.Duration) {
	s.timerMu.Lock()
	defer s.timerMu.Unlock()
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	if d > 0 {
		s.timer = time.AfterFunc(d, s.Stop)
	}
}

// --- gRPC service methods ---

func (s *Server) Get(_ context.Context, req *GetRequest) (*GetResponse, error) {
	s.dataMu.RLock()
	defer s.dataMu.RUnlock()
	val, ok := s.data[req.Key]
	if !ok {
		return nil, status.Error(codes.NotFound, "key not found")
	}
	return &GetResponse{Value: val}, nil
}

func (s *Server) List(_ context.Context, _ *ListRequest) (*ListResponse, error) {
	s.dataMu.RLock()
	defer s.dataMu.RUnlock()
	keys := make([]string, 0, len(s.data))
	for k := range s.data {
		if !isHistoryKey(k) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return &ListResponse{Keys: keys}, nil
}

func (s *Server) Set(_ context.Context, req *SetRequest) (*SetResponse, error) {
	s.dataMu.Lock()
	defer s.dataMu.Unlock()

	// Apply all mutations, recording history for replacements.
	for _, item := range req.Items {
		if old, exists := s.data[item.Key]; exists {
			n := nextHistorySuffix(s.data, item.Key)
			s.data[item.Key+"~"+strconv.Itoa(n)] = old
		}
		s.data[item.Key] = item.Value
	}

	if err := store.SaveData(s.data, s.backend, s.storeDir); err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("saving store: %v", err))
	}

	return &SetResponse{}, nil
}

func (s *Server) Delete(_ context.Context, req *DeleteRequest) (*DeleteResponse, error) {
	s.dataMu.Lock()
	defer s.dataMu.Unlock()

	// Verify all keys exist before mutating anything.
	for _, key := range req.Keys {
		if _, exists := s.data[key]; !exists {
			return nil, status.Error(codes.NotFound, "key not found: "+key)
		}
	}

	// Snapshot for rollback on save failure.
	snapshot := make(map[string]string)
	for _, key := range req.Keys {
		snapshot[key] = s.data[key]
		for _, hk := range historyKeys(s.data, key) {
			snapshot[hk] = s.data[hk]
		}
	}

	for _, key := range req.Keys {
		delete(s.data, key)
		for _, hk := range historyKeys(s.data, key) {
			delete(s.data, hk)
		}
	}

	if err := store.SaveData(s.data, s.backend, s.storeDir); err != nil {
		for k, v := range snapshot {
			s.data[k] = v
		}
		return nil, status.Error(codes.Internal, fmt.Sprintf("saving store: %v", err))
	}

	return &DeleteResponse{}, nil
}

func (s *Server) Passwd(_ context.Context, req *PasswdRequest) (*PasswdResponse, error) {
	s.dataMu.Lock()
	defer s.dataMu.Unlock()

	if !s.checkPassphrase(req.Passphrase) {
		return nil, status.Error(codes.PermissionDenied, ErrPassphraseRequired)
	}

	newBackend := s.newBackend(req.NewPassphrase)

	if err := store.SaveData(s.data, newBackend, s.storeDir); err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("saving store: %v", err))
	}

	s.passphrase = req.NewPassphrase
	s.backend = newBackend

	return &PasswdResponse{}, nil
}

func (s *Server) Rename(_ context.Context, req *RenameRequest) (*RenameResponse, error) {
	s.dataMu.Lock()
	defer s.dataMu.Unlock()

	if req.From == req.To {
		return nil, status.Error(codes.InvalidArgument, "source and destination are the same key")
	}

	val, exists := s.data[req.From]
	if !exists {
		return nil, status.Error(codes.NotFound, "key not found: "+req.From)
	}
	if _, exists := s.data[req.To]; exists {
		return nil, status.Error(codes.AlreadyExists, "key already exists: "+req.To)
	}

	// Collect history entries before mutating.
	histEntries := historyKeys(s.data, req.From)
	histValues := make(map[string]string, len(histEntries))
	for _, hk := range histEntries {
		histValues[hk] = s.data[hk]
	}

	s.data[req.To] = val
	delete(s.data, req.From)
	for _, hk := range histEntries {
		suffix := hk[len(req.From):] // "~N"
		s.data[req.To+suffix] = s.data[hk]
		delete(s.data, hk)
	}

	if err := store.SaveData(s.data, s.backend, s.storeDir); err != nil {
		// Rollback in-memory state.
		s.data[req.From] = val
		delete(s.data, req.To)
		for _, hk := range histEntries {
			suffix := hk[len(req.From):]
			delete(s.data, req.To+suffix)
			s.data[hk] = histValues[hk]
		}
		return nil, status.Error(codes.Internal, fmt.Sprintf("saving store: %v", err))
	}

	return &RenameResponse{}, nil
}

func (s *Server) SetAgentTTL(_ context.Context, req *SetAgentTTLRequest) (*SetAgentTTLResponse, error) {
	if req.Seconds < -1 {
		return nil, status.Error(codes.InvalidArgument, "TTL must be -1 (stop), 0 (infinite), or >0 (seconds)")
	}
	if req.Seconds == -1 {
		go s.Stop()
		return &SetAgentTTLResponse{}, nil
	}
	s.resetTimer(time.Duration(req.Seconds) * time.Second)
	return &SetAgentTTLResponse{}, nil
}

func (s *Server) History(_ context.Context, req *HistoryRequest) (*HistoryResponse, error) {
	s.dataMu.RLock()
	defer s.dataMu.RUnlock()

	hkeys := historyKeys(s.data, req.Key)
	n := len(hkeys)
	keys := make([]string, n)
	values := make([]string, n)
	// Return newest first (reverse order of ascending suffixes).
	for i, k := range hkeys {
		keys[n-1-i] = k
		values[n-1-i] = s.data[k]
	}
	return &HistoryResponse{Keys: keys, Values: values}, nil
}

func (s *Server) checkPassphrase(provided string) bool {
	return subtle.ConstantTimeCompare([]byte(s.passphrase), []byte(provided)) == 1
}

// --- History helpers ---

// isHistoryKey returns true if key is a history entry (contains '~').
func isHistoryKey(key string) bool {
	return strings.ContainsRune(key, '~')
}

// historyKeys returns all history entry keys for the given base key,
// sorted ascending by their numeric suffix (oldest first).
func historyKeys(data map[string]string, base string) []string {
	prefix := base + "~"
	var keys []string
	for k := range data {
		if strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		return parseHistorySuffix(keys[i]) < parseHistorySuffix(keys[j])
	})
	return keys
}

// parseHistorySuffix returns the numeric suffix N from "KEY~N", or -1 if unparseable.
func parseHistorySuffix(key string) int {
	i := strings.LastIndexByte(key, '~')
	if i < 0 {
		return -1
	}
	n, err := strconv.Atoi(key[i+1:])
	if err != nil {
		return -1
	}
	return n
}

// nextHistorySuffix returns the next suffix for a new history entry.
func nextHistorySuffix(data map[string]string, base string) int {
	keys := historyKeys(data, base)
	if len(keys) == 0 {
		return 1
	}
	return parseHistorySuffix(keys[len(keys)-1]) + 1
}
