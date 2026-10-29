package integration_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	agentsqlite "github.com/z-chenhao/collie/internal/agent/sqlite"
	"github.com/z-chenhao/collie/internal/event/testbroker"
)

func TestRelayCLI(t *testing.T) {
	binary := os.Getenv("COLLIE_TEST_BINARY")
	if binary == "" {
		t.Skip("real CLI is driven by make verify after make build")
	}
	broker := testbroker.New(t)
	url := broker.URL()
	broker.Stop()
	root := t.TempDir()
	dataRoot := filepath.Join(root, "data")
	configuration := filepath.Join(root, "config.yaml")
	text := fmt.Sprintf("version: 1\napp:\n  data_root: %q\nnats:\n  url: %q\n  stream_prefix: test\n  timeout: 50ms\n  retry_interval: 10ms\n  ack_wait: 100ms\n", dataRoot, url)
	checkErr(t, os.WriteFile(configuration, []byte(text), 0600))
	childCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(childCtx, binary, "-config", configuration, "relay")
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "COLLIE_") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Dir = root
	out, err := cmd.StdoutPipe()
	checkErr(t, err)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	checkErr(t, cmd.Start())
	waited := false
	defer func() {
		if !waited {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()
	ready := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(out)
		if scanner.Scan() {
			ready <- scanner.Text()
		} else {
			ready <- ""
		}
	}()
	var diagnostics map[string]any
	select {
	case line := <-ready:
		checkErr(t, json.Unmarshal([]byte(line), &diagnostics))
	case <-childCtx.Done():
		t.Fatal("relay CLI startup timed out")
	}
	if diagnostics["readiness"] != "setup_required" || diagnostics["agent_ingress_enabled"] != false {
		t.Fatal("relay enabled model ingress")
	}
	store, err := agentsqlite.Open(context.Background(), dataRoot)
	checkErr(t, err)
	defer store.Close()
	pending, err := store.Events().Pending(context.Background(), 100)
	checkErr(t, err)
	if len(pending) != 1 {
		t.Fatal("offline relay lost bootstrap event")
	}
	if cmd.ProcessState != nil {
		t.Fatal("relay exited on broker outage")
	}
	broker.Start(t)
	bus := connect(t, broker)
	sub := subscribe(t, bus, "cli-evidence")
	delivery := receive(t, sub)
	if delivery.Event().ID() != pending[0].ID() {
		t.Fatal("relay changed frozen event ID")
	}
	// Observe the SQLite marker through the authoritative database boundary.
	deadline := time.Now().Add(5 * time.Second)
	for stateCount(t, store, "SELECT count(*) FROM event_outbox WHERE published_at IS NULL") != 0 {
		if time.Now().After(deadline) {
			t.Fatal("relay did not mark broker acceptance")
		}
		time.Sleep(5 * time.Millisecond)
	}
	ack(t, delivery)
	checkErr(t, cmd.Process.Signal(syscall.SIGTERM))
	checkErr(t, cmd.Wait())
	waited = true
	if stderr.Len() != 0 {
		t.Fatalf("relay shutdown stderr: %s", stderr.String())
	}
	status := exec.CommandContext(childCtx, binary, "-config", configuration, "status")
	status.Env = cmd.Env
	status.Dir = root
	raw, err := status.Output()
	checkErr(t, err)
	var after map[string]any
	checkErr(t, json.Unmarshal(raw, &after))
	if after["agent_id"] != diagnostics["agent_id"] || after["readiness"] != "setup_required" {
		t.Fatal("relay changed persisted Agent identity/readiness")
	}
	report := map[string]any{"result": "passed", "checks": []string{"isolated CLI launch while broker unavailable", "setup_required and closed ingress preserved", "same frozen event published when owned broker recovers", "SQLite delivery marker observed", "clean SIGTERM and status restart preserve identity"}, "event_id": delivery.Event().ID(), "diagnostics": diagnostics}
	raw, err = json.MarshalIndent(report, "", "  ")
	checkErr(t, err)
	t.Log(string(raw))
	if path := os.Getenv("COLLIE_EVENT_EVIDENCE"); path != "" {
		checkErr(t, os.WriteFile(filepath.Join(filepath.Dir(path), "relay.json"), append(raw, '\n'), 0600))
	}
}
