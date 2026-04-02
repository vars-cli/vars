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
	dialTimeout           = 2 * time.Second
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

	resp, err := NewVarsClient(conn).Get(ctx, &GetRequest{Key: key})
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

	resp, err := NewVarsClient(conn).List(ctx, &ListRequest{})
	if err != nil {
		return nil, statusMsg(err)
	}
	return resp.Keys, nil
}

// Set stores one or more key-value pairs via the agent.
func Set(sockPath string, items []SetItem) error {
	conn, err := newConn(sockPath)
	if err != nil {
		return fmt.Errorf("connecting to agent: %w", err)
	}
	defer conn.Close()

	ctx, cancel := ctx30s()
	defer cancel()

	protoItems := make([]*SetItem, len(items))
	for i := range items {
		protoItems[i] = &items[i]
	}
	_, err = NewVarsClient(conn).Set(ctx, &SetRequest{Items: protoItems})
	return statusMsg(err)
}

// Delete removes one or more keys and their history via the agent.
func Delete(sockPath string, keys []string) error {
	conn, err := newConn(sockPath)
	if err != nil {
		return fmt.Errorf("connecting to agent: %w", err)
	}
	defer conn.Close()

	ctx, cancel := ctx30s()
	defer cancel()

	_, err = NewVarsClient(conn).Delete(ctx, &DeleteRequest{Keys: keys})
	return statusMsg(err)
}

// History retrieves the value history for a key, newest first.
// Returns parallel slices of store key names (e.g. "RPC_URL~3") and their values.
func History(sockPath, key string) (keys, values []string, err error) {
	conn, connErr := newConn(sockPath)
	if connErr != nil {
		return nil, nil, fmt.Errorf("connecting to agent: %w", connErr)
	}
	defer conn.Close()

	ctx, cancel := ctx30s()
	defer cancel()

	resp, rpcErr := NewVarsClient(conn).History(ctx, &HistoryRequest{Key: key})
	if rpcErr != nil {
		return nil, nil, statusMsg(rpcErr)
	}
	return resp.Keys, resp.Values, nil
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

	_, err = NewVarsClient(conn).Passwd(ctx, &PasswdRequest{Passphrase: oldPass, NewPassphrase: newPass})
	return statusMsg(err)
}

// Rename atomically renames a key in the store.
func Rename(sockPath, from, to string) error {
	conn, err := newConn(sockPath)
	if err != nil {
		return fmt.Errorf("connecting to agent: %w", err)
	}
	defer conn.Close()

	ctx, cancel := ctx30s()
	defer cancel()

	_, err = NewVarsClient(conn).Rename(ctx, &RenameRequest{From: from, To: to})
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

	_, err = NewVarsClient(conn).SetAgentTTL(ctx, &SetAgentTTLRequest{Seconds: seconds})
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
