package integration_test

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentsqlite "github.com/z-chenhao/collie/internal/agent/sqlite"
	"github.com/z-chenhao/collie/internal/event"
	eventnats "github.com/z-chenhao/collie/internal/event/nats"
	eventsqlite "github.com/z-chenhao/collie/internal/event/sqlite"
	"github.com/z-chenhao/collie/internal/event/testbroker"
)

func checkErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func stateCount(t *testing.T, s *agentsqlite.Store, query string) int {
	t.Helper()
	var n int
	checkErr(t, s.Transaction(context.Background(), func(tx *sql.Tx) error { return tx.QueryRow(query).Scan(&n) }))
	return n
}
func domainEffect(ctx context.Context, tx *sql.Tx, _ event.CloudEvent) error {
	_, err := tx.ExecContext(ctx, `UPDATE effect SET value=value+1`)
	return err
}

// The child test process stops at an observable boundary. The parent kills this
// exact process with SIGKILL; none of its deferred cleanup runs.
func TestCrashWorker(t *testing.T) {
	phase := os.Getenv("COLLIE_CRASH_PHASE")
	if phase == "" {
		return
	}
	ctx := context.Background()
	s, err := agentsqlite.Open(ctx, os.Getenv("COLLIE_CRASH_ROOT"))
	checkErr(t, err)
	defer s.Close()
	checkpoint := func() { fmt.Println("COLLIE_CHECKPOINT"); select {} }
	if phase == "before-domain-commit" || phase == "after-domain-commit" {
		err := s.Transaction(ctx, func(tx *sql.Tx) error {
			if err := domainEffect(ctx, tx, event.CloudEvent{}); err != nil {
				return err
			}
			if err := eventsqlite.Enqueue(ctx, tx, testEvent(t)); err != nil {
				return err
			}
			if phase == "before-domain-commit" {
				checkpoint()
			}
			return nil
		})
		checkErr(t, err)
		checkpoint()
		return
	}
	bus, err := eventnats.Open(ctx, options(os.Getenv("COLLIE_CRASH_URL")))
	checkErr(t, err)
	defer bus.Close()
	if phase == "before-publish" || phase == "after-publish-before-mark" {
		wrapper := publishBarrier{EventBus: bus, before: phase == "before-publish", checkpoint: checkpoint}
		_, err := (event.Publisher{Outbox: s.Events(), Bus: wrapper}).Flush(ctx, 100)
		checkErr(t, err)
		t.Fatal("checkpoint not reached")
	}
	sub := subscribe(t, bus, "crash-consumer")
	d := receive(t, sub)
	effect := domainEffect
	if phase == "before-inbox-commit" {
		effect = func(ctx context.Context, tx *sql.Tx, e event.CloudEvent) error {
			if err := domainEffect(ctx, tx, e); err != nil {
				return err
			}
			checkpoint()
			return nil
		}
	}
	if phase == "after-inbox-commit-before-ack" || phase == "after-ack" {
		d = ackBarrier{Delivery: d, after: phase == "after-ack", checkpoint: checkpoint}
	}
	_, err = s.Events().Handle(ctx, "crash-consumer", d, effect)
	checkErr(t, err)
	t.Fatal("checkpoint not reached")
}

type publishBarrier struct {
	event.EventBus
	before     bool
	checkpoint func()
}

func (b publishBarrier) Publish(ctx context.Context, e event.CloudEvent) error {
	if b.before {
		b.checkpoint()
	}
	if err := b.EventBus.Publish(ctx, e); err != nil {
		return err
	}
	b.checkpoint()
	return nil
}

type ackBarrier struct {
	event.Delivery
	after      bool
	checkpoint func()
}

func (b ackBarrier) Ack(ctx context.Context) error {
	if !b.after {
		b.checkpoint()
	}
	if err := b.Delivery.Ack(ctx); err != nil {
		return err
	}
	b.checkpoint()
	return nil
}

func killAtCheckpoint(t *testing.T, phase, root, url string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCrashWorker$", "-test.timeout=12s")
	cmd.Env = append(os.Environ(), "COLLIE_CRASH_PHASE="+phase, "COLLIE_CRASH_ROOT="+root, "COLLIE_CRASH_URL="+url)
	stdout, err := cmd.StdoutPipe()
	checkErr(t, err)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	checkErr(t, cmd.Start())
	killed := false
	defer func() {
		if !killed {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()
	ready := make(chan bool, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if scanner.Text() == "COLLIE_CHECKPOINT" {
				ready <- true
				return
			}
		}
		ready <- false
	}()
	select {
	case ok := <-ready:
		if !ok {
			t.Fatalf("child never reached %s", phase)
		}
	case <-ctx.Done():
		t.Fatalf("child timed out at %s", phase)
	}
	checkErr(t, cmd.Process.Kill())
	err = cmd.Wait()
	killed = true
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ProcessState.Success() {
		t.Fatalf("child was not killed: %v", err)
	}
}

func TestProcessCrashBoundaries(t *testing.T) {
	phases := []string{"before-domain-commit", "after-domain-commit", "before-publish", "after-publish-before-mark", "before-inbox-commit", "after-inbox-commit-before-ack", "after-ack"}
	evidence := make([]map[string]any, 0, len(phases))
	for _, phase := range phases {
		t.Run(phase, func(t *testing.T) {
			broker := testbroker.New(t)
			bus := connect(t, broker)
			root := t.TempDir()
			s, err := agentsqlite.Open(context.Background(), root)
			checkErr(t, err)
			defer func() { _ = s.Close() }()
			checkErr(t, s.Transaction(context.Background(), func(tx *sql.Tx) error {
				_, err := tx.Exec(`CREATE TABLE effect (value INTEGER NOT NULL); INSERT INTO effect VALUES (0)`)
				return err
			}))
			inboxPhase := strings.Contains(phase, "inbox") || phase == "after-ack"
			if inboxPhase {
				n, err := (event.Publisher{Outbox: s.Events(), Bus: bus}).Flush(context.Background(), 100)
				checkErr(t, err)
				if n != 1 {
					t.Fatal("bootstrap event not queued")
				}
			}
			checkErr(t, s.Close())
			killAtCheckpoint(t, phase, root, broker.URL())
			s, err = agentsqlite.Open(context.Background(), root)
			checkErr(t, err)
			value := stateCount(t, s, "SELECT value FROM effect")
			pending := stateCount(t, s, "SELECT count(*) FROM event_outbox WHERE published_at IS NULL")
			inbox := stateCount(t, s, "SELECT count(*) FROM event_inbox")
			switch phase {
			case "before-domain-commit":
				if value != 0 || pending != 1 {
					t.Fatal("uncommitted transaction survived kill")
				}
			case "after-domain-commit":
				if value != 1 || pending != 2 {
					t.Fatal("committed transaction lost after kill")
				}
			case "before-publish", "after-publish-before-mark":
				if pending != 1 {
					t.Fatal("publish ambiguity lost intent")
				}
			case "before-inbox-commit":
				if value != 0 || inbox != 0 {
					t.Fatal("incomplete effect survived kill")
				}
			case "after-inbox-commit-before-ack", "after-ack":
				if value != 1 || inbox != 1 {
					t.Fatal("committed inbox/effect lost")
				}
			}
			if phase == "before-publish" || phase == "after-publish-before-mark" {
				n, err := (event.Publisher{Outbox: s.Events(), Bus: bus}).Flush(context.Background(), 100)
				checkErr(t, err)
				if n != 1 {
					t.Fatal("restart failed to publish pending intent")
				}
				sub := subscribe(t, bus, "post-publish")
				deliveries := 1
				if phase == "after-publish-before-mark" {
					deliveries = 2
				}
				for range deliveries {
					d := receive(t, sub)
					_, err := s.Events().Handle(context.Background(), "post-publish", d, domainEffect)
					checkErr(t, err)
				}
				if stateCount(t, s, "SELECT value FROM effect") != 1 || stateCount(t, s, "SELECT count(*) FROM event_inbox") != 1 {
					t.Fatal("publish redelivery repeated effect")
				}
			}
			if inboxPhase {
				sub := subscribe(t, bus, "crash-consumer")
				if phase == "after-ack" {
					ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
					defer cancel()
					if _, err := sub.Next(ctx); err == nil {
						t.Fatal("acknowledged event repeated after restart")
					}
				} else {
					d := receive(t, sub)
					handled, err := s.Events().Handle(context.Background(), "crash-consumer", d, domainEffect)
					checkErr(t, err)
					if handled != (phase == "before-inbox-commit") {
						t.Fatal("incorrect redelivery dedup decision")
					}
				}
				if stateCount(t, s, "SELECT value FROM effect") != 1 {
					t.Fatal("restart repeated/lost handled effect")
				}
			}
			record := map[string]any{"phase": phase, "after_kill": map[string]int{"domain_value": value, "pending_outbox": pending, "inbox_rows": inbox}, "result": "passed"}
			evidence = append(evidence, record)
			raw, _ := json.Marshal(record)
			t.Log(string(raw))
		})
	}
	if path := os.Getenv("COLLIE_EVENT_EVIDENCE"); path != "" && !t.Failed() {
		raw, err := json.MarshalIndent(map[string]any{"result": "passed", "fault_boundaries": evidence}, "", "  ")
		checkErr(t, err)
		checkErr(t, os.MkdirAll(filepath.Dir(path), 0700))
		checkErr(t, os.WriteFile(path, append(raw, '\n'), 0600))
	}
}
