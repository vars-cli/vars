package agent

import (
	"context"
	"fmt"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// unixAddr converts a socket path to a gRPC unix:// URI.
// "unix://" + "/abs/path" = "unix:///abs/path" (3 slashes for absolute paths).
func unixAddr(sockPath string) string { return "unix://" + sockPath }

const (
	dialTimeout          = 2 * time.Second
	ErrPassphraseRequired = "passphrase required"
)

func newConn(sockPath string) (*grpc.ClientConn, error) {
	return grpc.NewClient(unixAddr(sockPath),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
}

// statusMsg extracts the plain message from a gRPC status error.
func statusMsg(err error) error {
	if err == nil {
		return nil
	}
	if s, ok := status.FromError(err); ok {
		return fmt.Errorf("%s", s.Message())
	}
	return err
}

func ctx30s() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Second)
}

func ctx2s() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), dialTimeout)
}

// Get retrieves a value from the agent.
func Get(sockPath, key string) (string, error) {
	conn, err := newConn(sockPath)
	if err != nil {
		return "", fmt.Errorf("connecting to agent: %w", err)
	}
	defer conn.Close()

	ctx, cancel := ctx30s()
	defer cancel()

	resp, err := NewSecretsClient(conn).Get(ctx, &GetRequest{Key: key})
	if err != nil {
		return "", statusMsg(err)
	}
	return resp.Value, nil
}

// List retrieves all keys from the agent.
func List(sockPath string) ([]string, error) {
	conn, err := newConn(sockPath)
	if err != nil {
		return nil, fmt.Errorf("connecting to agent: %w", err)
	}
	defer conn.Close()

	ctx, cancel := ctx30s()
	defer cancel()

	resp, err := NewSecretsClient(conn).List(ctx, &ListRequest{})
	if err != nil {
		return nil, statusMsg(err)
	}
	return resp.Keys, nil
}

// Set stores a key-value pair via the agent.
// Passphrase is required when overwriting an existing key.
func Set(sockPath, key, value, passphrase string) error {
	conn, err := newConn(sockPath)
	if err != nil {
		return fmt.Errorf("connecting to agent: %w", err)
	}
	defer conn.Close()

	ctx, cancel := ctx30s()
	defer cancel()

	_, err = NewSecretsClient(conn).Set(ctx, &SetRequest{Key: key, Value: value, Passphrase: passphrase})
	return statusMsg(err)
}

// Delete removes a key via the agent. Passphrase is always required.
func Delete(sockPath, key, passphrase string) error {
	conn, err := newConn(sockPath)
	if err != nil {
		return fmt.Errorf("connecting to agent: %w", err)
	}
	defer conn.Close()

	ctx, cancel := ctx30s()
	defer cancel()

	_, err = NewSecretsClient(conn).Delete(ctx, &DeleteRequest{Key: key, Passphrase: passphrase})
	return statusMsg(err)
}

// Passwd changes the store passphrase via the agent.
func Passwd(sockPath, oldPass, newPass string) error {
	conn, err := newConn(sockPath)
	if err != nil {
		return fmt.Errorf("connecting to agent: %w", err)
	}
	defer conn.Close()

	ctx, cancel := ctx30s()
	defer cancel()

	_, err = NewSecretsClient(conn).Passwd(ctx, &PasswdRequest{Passphrase: oldPass, NewPassphrase: newPass})
	return statusMsg(err)
}

// Rename atomically renames a key in the store. Passphrase is always required.
func Rename(sockPath, from, to, passphrase string) error {
	conn, err := newConn(sockPath)
	if err != nil {
		return fmt.Errorf("connecting to agent: %w", err)
	}
	defer conn.Close()

	ctx, cancel := ctx30s()
	defer cancel()

	_, err = NewSecretsClient(conn).Rename(ctx, &RenameRequest{From: from, To: to, Passphrase: passphrase})
	return statusMsg(err)
}

// SetAgentTTL adjusts the agent's lifetime.
// seconds: -1 = stop immediately, 0 = no expiry, >0 = lifetime in seconds.
func SetAgentTTL(sockPath string, seconds int64) error {
	conn, err := newConn(sockPath)
	if err != nil {
		return fmt.Errorf("connecting to agent: %w", err)
	}
	defer conn.Close()

	ctx, cancel := ctx2s()
	defer cancel()

	_, err = NewSecretsClient(conn).SetAgentTTL(ctx, &SetAgentTTLRequest{Seconds: seconds})
	return statusMsg(err)
}

// Stop signals the agent to wipe memory and exit immediately.
func Stop(sockPath string) error {
	return SetAgentTTL(sockPath, -1)
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
