package agent

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sort"
	"sync"
	"syscall"
	"time"

	"crypto/subtle"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/brickpop/secrets/internal/crypto"
	"github.com/brickpop/secrets/internal/store"
)

// Server holds decrypted store data in memory and serves it over a Unix socket via gRPC.
type Server struct {
	UnimplementedSecretsServer

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
	RegisterSecretsServer(s.grpcSrv, s)

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
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return &ListResponse{Keys: keys}, nil
}

func (s *Server) Set(_ context.Context, req *SetRequest) (*SetResponse, error) {
	s.dataMu.Lock()
	defer s.dataMu.Unlock()

	if _, exists := s.data[req.Key]; exists {
		if !s.checkPassphrase(req.Passphrase) {
			return nil, status.Error(codes.PermissionDenied, ErrPassphraseRequired)
		}
	}

	s.data[req.Key] = req.Value

	if err := store.SaveData(s.data, s.backend, s.storeDir); err != nil {
		delete(s.data, req.Key)
		return nil, status.Error(codes.Internal, fmt.Sprintf("saving store: %v", err))
	}

	return &SetResponse{}, nil
}

func (s *Server) Delete(_ context.Context, req *DeleteRequest) (*DeleteResponse, error) {
	s.dataMu.Lock()
	defer s.dataMu.Unlock()

	if !s.checkPassphrase(req.Passphrase) {
		return nil, status.Error(codes.PermissionDenied, ErrPassphraseRequired)
	}

	if _, exists := s.data[req.Key]; !exists {
		return nil, status.Error(codes.NotFound, "key not found")
	}

	delete(s.data, req.Key)

	if err := store.SaveData(s.data, s.backend, s.storeDir); err != nil {
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

	if !s.checkPassphrase(req.Passphrase) {
		return nil, status.Error(codes.PermissionDenied, ErrPassphraseRequired)
	}

	val, exists := s.data[req.From]
	if !exists {
		return nil, status.Error(codes.NotFound, "key not found: "+req.From)
	}
	if _, exists := s.data[req.To]; exists {
		return nil, status.Error(codes.AlreadyExists, "key already exists: "+req.To)
	}

	s.data[req.To] = val
	delete(s.data, req.From)

	if err := store.SaveData(s.data, s.backend, s.storeDir); err != nil {
		// Rollback in-memory state
		s.data[req.From] = val
		delete(s.data, req.To)
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

func (s *Server) checkPassphrase(provided string) bool {
	return subtle.ConstantTimeCompare([]byte(s.passphrase), []byte(provided)) == 1
}
