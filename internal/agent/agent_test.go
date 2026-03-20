package agent

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func startTestServer(t *testing.T, data map[string]string, ttl time.Duration) (string, *Server) {
	t.Helper()
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "agent.sock")
	srv := NewServer(data, sockPath)

	ready := make(chan struct{})
	go func() {
		close(ready)
		srv.Start(ttl)
	}()
	<-ready

	// Wait for socket to be ready
	for i := 0; i < 50; i++ {
		if IsRunning(sockPath) {
			return sockPath, srv
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("server did not start in time")
	return "", nil
}

func TestGetExisting(t *testing.T) {
	sockPath, srv := startTestServer(t, map[string]string{
		"KEY_A": "value_a",
		"KEY_B": "value_b",
	}, 0)
	defer srv.Stop()

	val, err := Get(sockPath, "KEY_A")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != "value_a" {
		t.Fatalf("Get = %q, want %q", val, "value_a")
	}
}

func TestGetMissing(t *testing.T) {
	sockPath, srv := startTestServer(t, map[string]string{
		"KEY_A": "value_a",
	}, 0)
	defer srv.Stop()

	_, err := Get(sockPath, "NONEXISTENT")
	if err == nil {
		t.Fatal("Get missing key should fail")
	}
}

func TestList(t *testing.T) {
	sockPath, srv := startTestServer(t, map[string]string{
		"ZEBRA": "z",
		"ALPHA": "a",
		"MIKE":  "m",
	}, 0)
	defer srv.Stop()

	keys, err := List(sockPath)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	expected := []string{"ALPHA", "MIKE", "ZEBRA"}
	if len(keys) != len(expected) {
		t.Fatalf("List len = %d, want %d", len(keys), len(expected))
	}
	for i, k := range keys {
		if k != expected[i] {
			t.Fatalf("List[%d] = %q, want %q", i, k, expected[i])
		}
	}
}

func TestListEmpty(t *testing.T) {
	sockPath, srv := startTestServer(t, map[string]string{}, 0)
	defer srv.Stop()

	keys, err := List(sockPath)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(keys) != 0 {
		t.Fatalf("List empty = %v, want empty", keys)
	}
}

func TestStop(t *testing.T) {
	sockPath, srv := startTestServer(t, map[string]string{
		"KEY": "value",
	}, 0)
	_ = srv // don't defer Stop — we'll stop via client

	err := Stop(sockPath)
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// Agent should be gone
	time.Sleep(50 * time.Millisecond)
	if IsRunning(sockPath) {
		t.Fatal("agent should not be running after Stop")
	}
}

func TestTTLExpiry(t *testing.T) {
	sockPath, _ := startTestServer(t, map[string]string{
		"KEY": "value",
	}, 200*time.Millisecond)

	// Should be running initially
	if !IsRunning(sockPath) {
		t.Fatal("agent should be running")
	}

	// Wait for TTL
	time.Sleep(400 * time.Millisecond)

	if IsRunning(sockPath) {
		t.Fatal("agent should have stopped after TTL")
	}
}

func TestTTLZeroNoExpiry(t *testing.T) {
	sockPath, srv := startTestServer(t, map[string]string{
		"KEY": "value",
	}, 0)
	defer srv.Stop()

	// Should still be running after some time
	time.Sleep(200 * time.Millisecond)
	if !IsRunning(sockPath) {
		t.Fatal("agent with TTL=0 should still be running")
	}
}

func TestConcurrentClients(t *testing.T) {
	sockPath, srv := startTestServer(t, map[string]string{
		"KEY": "value",
	}, 0)
	defer srv.Stop()

	var wg sync.WaitGroup
	errors := make(chan error, 20)

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			val, err := Get(sockPath, "KEY")
			if err != nil {
				errors <- err
				return
			}
			if val != "value" {
				errors <- fmt.Errorf("got %q, want %q", val, "value")
			}
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Fatalf("concurrent client error: %v", err)
	}
}

func TestIsRunning_NotRunning(t *testing.T) {
	if IsRunning("/nonexistent/socket.sock") {
		t.Fatal("IsRunning should be false for nonexistent socket")
	}
}

func TestUnknownOp(t *testing.T) {
	sockPath, srv := startTestServer(t, map[string]string{}, 0)
	defer srv.Stop()

	resp, err := roundTrip(sockPath, &Request{Op: "invalid"})
	if err != nil {
		t.Fatalf("roundTrip: %v", err)
	}
	if resp.OK {
		t.Fatal("unknown op should return ok=false")
	}
}

// ensure fmt is used
var _ = fmt.Errorf
