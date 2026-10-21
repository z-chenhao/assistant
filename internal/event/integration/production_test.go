package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	nat "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	agentsqlite "github.com/z-chenhao/erii/internal/agent/sqlite"
	"github.com/z-chenhao/erii/internal/event"
	eventnats "github.com/z-chenhao/erii/internal/event/nats"
	"github.com/z-chenhao/erii/internal/event/testbroker"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestBrokerFixture is a test-only, owned JetStream peer for Python surfaces.
// Its stdin lifetime and JSON handshake are consumed by event_broker.py.
func TestBrokerFixture(t *testing.T) {
	if os.Getenv("ERII_BROKER_FIXTURE") != "1" {
		t.Skip("owned fixture only")
	}
	// Interactive scenarios share this peer but own separate Stream prefixes.
	// Six namespaces reserve about 6.4 GiB; actual fixture writes remain tiny.
	b := testbroker.NewWithStoreLimit(t, 8<<30)
	if e := json.NewEncoder(os.Stdout).Encode(map[string]any{"version": 1, "url": b.URL()}); e != nil {
		t.Fatal(e)
	}
	if _, e := io.Copy(io.Discard, os.Stdin); e != nil {
		t.Fatal(e)
	}
}

type tokenStore struct {
	value string
	reads int
}

func (s *tokenStore) Get(context.Context, string) ([]byte, error) {
	s.reads++
	return []byte(s.value), nil
}
func TestSecretStoreBrokerAuthBoundsAndPoisonIsolation(t *testing.T) {
	ctx := context.Background()
	token := "PRIVATE_BROKER_TOKEN_29"
	broker := testbroker.NewAuthenticated(t, token)
	opts := options(broker.URL())
	opts.PoisonAttempts = 2
	opts.TokenRef = "test:broker"
	opts.AckWait = 20 * time.Millisecond
	if _, e := eventnats.Open(ctx, opts); e == nil || strings.Contains(e.Error(), token) {
		t.Fatal("missing SecretStore accepted or leaked", e)
	}
	wrong := &tokenStore{value: "WRONG_PRIVATE_TOKEN"}
	if _, e := eventnats.OpenWithSecrets(ctx, opts, wrong); e == nil || strings.Contains(e.Error(), wrong.value) {
		t.Fatal("bad credential accepted or leaked", e)
	}
	secrets := &tokenStore{value: token}
	bus, e := eventnats.OpenWithSecrets(ctx, opts, secrets)
	checkErr(t, e)
	defer bus.Close()
	if secrets.reads != 1 {
		t.Fatal("credential not resolved once")
	}
	conn, e := nat.Connect(broker.URL(), nat.Token(token))
	checkErr(t, e)
	defer conn.Close()
	js, e := jetstream.New(conn)
	checkErr(t, e)
	_, e = js.Publish(ctx, "test.v1.events.poison", []byte(`PRIVATE_BODY_AND_TOKEN`))
	checkErr(t, e)
	good := testEvent(t)
	checkErr(t, bus.Publish(ctx, good))
	sub := subscribe(t, bus, "runtime_wake_v1")
	bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	goodDelivered := false
	quarantined := false
	for !goodDelivered || !quarantined {
		d, e := sub.Next(bounded)
		if errors.Is(e, event.ErrPoison) {
			h, e := bus.Health(ctx)
			checkErr(t, e)
			quarantined = h.Dead > 0
			continue
		}
		checkErr(t, e)
		if d.Event().ID() != good.ID() {
			t.Fatal("poison entered domain")
		}
		ack(t, d)
		goodDelivered = true
	}
	h, e := bus.Health(ctx)
	checkErr(t, e)
	if h.Dead != 1 || h.MaxBytes != 1<<30 || h.MaxAgeSeconds != 604800 || h.AckPending != 0 {
		t.Fatal("bounds or poison recovery", h)
	}
	stream, e := js.Stream(ctx, "test_dlq_v1")
	checkErr(t, e)
	raw, e := stream.GetMsg(ctx, 1)
	checkErr(t, e)
	if strings.Contains(string(raw.Data), "PRIVATE") || strings.Contains(string(raw.Data), token) {
		t.Fatal("DLQ leaked body")
	}
	var v map[string]any
	checkErr(t, json.Unmarshal(raw.Data, &v))
	if v["code"] != "POISON_THRESHOLD" || v["attempts"] != float64(2) {
		t.Fatal("wrong receipt", v)
	}
	// Durable quarantine survives restart; it does not generate another domain
	// message or block a later valid aggregate.
	sub.Close()
	bus.Close()
	broker.Stop()
	broker.Start(t)
	bus, e = eventnats.OpenWithSecrets(ctx, opts, secrets)
	checkErr(t, e)
	defer bus.Close()
	h, e = bus.Health(ctx)
	checkErr(t, e)
	if h.Dead != 1 {
		t.Fatal("DLQ not durable")
	}
	second := testEvent(t)
	checkErr(t, bus.Publish(ctx, second))
	sub = subscribe(t, bus, "runtime_wake_v1")
	if d := receive(t, sub); d.Event().ID() != second.ID() {
		t.Fatal("poison repeated after restart")
	} else {
		ack(t, d)
	}
}

func TestRemoteBrokerFailsClosedAndBoundsCannotDrift(t *testing.T) {
	for _, url := range []string{"nats://example.com:4222", "tls://example.com:4222", "nats://10.0.0.1:4222"} {
		o := options(url)
		if o.Validate() == nil {
			t.Fatal("unprotected remote accepted", url)
		}
	}
	o := options("tls://example.com:4222")
	o.TokenRef = "keychain:broker"
	checkErr(t, o.Validate())
	broker := testbroker.New(t)
	bus := connect(t, broker)
	o = options(broker.URL())
	o.MaxBytes = 2 << 30
	if other, e := eventnats.Open(context.Background(), o); e == nil {
		other.Close()
		t.Fatal("silently replaced retention configuration")
	}
	bus.Close()
}

func TestUnboundedBrokerUpgradePreservesRetainedEvents(t *testing.T) {
	ctx := context.Background()
	broker := testbroker.New(t)
	conn, e := nat.Connect(broker.URL())
	checkErr(t, e)
	defer conn.Close()
	js, e := jetstream.New(conn)
	checkErr(t, e)
	_, e = js.CreateStream(ctx, jetstream.StreamConfig{Name: "test_events_v1", Subjects: []string{"test.v1.events.>"}, Storage: jetstream.FileStorage, Retention: jetstream.LimitsPolicy, Discard: jetstream.DiscardNew})
	checkErr(t, e)
	_, e = js.Publish(ctx, "test.v1.events.retained", []byte("retained"))
	checkErr(t, e)
	if b, e := eventnats.Open(ctx, options(broker.URL())); e == nil {
		b.Close()
		t.Fatal("old unbounded stream silently changed")
	}
	bus, e := eventnats.Upgrade(ctx, options(broker.URL()), nil)
	checkErr(t, e)
	defer bus.Close()
	h, e := bus.Health(ctx)
	checkErr(t, e)
	if h.Events != 1 || h.MaxBytes != 1<<30 {
		t.Fatal("upgrade lost retained event", h)
	}
}

// This is part of make verify's maintained real-binary/NATS gate. No online
// account or test-only execution loop is involved.
func TestProductionLifecycleCLI(t *testing.T) {
	binary := os.Getenv("ERII_TEST_BINARY")
	if binary == "" {
		t.Skip("real binary supplied by make verify")
	}
	ctx := context.Background()
	broker := testbroker.New(t)
	root := t.TempDir()
	data := filepath.Join(root, "data")
	cfg := filepath.Join(root, "config.yaml")
	node, e := exec.LookPath("node")
	checkErr(t, e)
	script, e := filepath.Abs("../../../runtime/javascript/src/sidecar.mjs")
	checkErr(t, e)
	checkErr(t, os.WriteFile(cfg, []byte(fmt.Sprintf("version: 1\napp:\n  data_root: %q\nnats:\n  url: %q\n  stream_prefix: test\n  timeout: 100ms\n  retry_interval: 10ms\n  ack_wait: 20ms\n  poison_attempts: 2\nmemory:\n  encryption_key_ref: env:ERII_LIFECYCLE_MEMORY\njavascript:\n  command: %q\n  script: %q\n", data, broker.URL(), node, script)), 0600))
	env := []string{}
	for _, v := range os.Environ() {
		if !strings.HasPrefix(v, "ERII_") {
			env = append(env, v)
		}
	}
	env = append(env, "ERII_LIFECYCLE_MEMORY=MMMMMMMMMMMMMMMMMMMMMMMMMMMMMMMM")
	args := []string{"-config", cfg, "-test-bridge", "main"}
	command := func(arguments ...string) map[string]any {
		t.Helper()
		cmd := exec.Command(binary, append(append([]string{}, args...), arguments...)...)
		cmd.Env = env
		cmd.Stdin = strings.NewReader("PRIVATE_LIFECYCLE_29")
		raw, e := cmd.CombinedOutput()
		if e != nil {
			t.Fatalf("CLI %v: %v %s", arguments, e, raw)
		}
		var result map[string]any
		checkErr(t, json.Unmarshal(raw, &result))
		return result
	}
	status := command("status")
	store, e := agentsqlite.Open(ctx, data)
	checkErr(t, e)
	defer store.Close()
	var stderr bytes.Buffer
	child := exec.Command(binary, append(append([]string{}, args...), "-test-events", "automation", "work")...)
	child.Env = env
	child.Stderr = &stderr
	checkErr(t, child.Start())
	waited := false
	defer func() {
		if !waited {
			child.Process.Kill()
			child.Wait()
			if stderr.Len() > 0 {
				t.Log("owned runtime stderr:", stderr.String())
			}
		}
	}()
	wait := func(name string, condition func() bool) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for !condition() {
			if time.Now().After(deadline) {
				t.Fatal("lifecycle did not reach " + name)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	wait("setup surface", func() bool {
		raw, _ := os.ReadFile(filepath.Join(data, "logs", "app.jsonl"))
		return bytes.Contains(raw, []byte("surface_open"))
	})
	connection, e := nat.Connect(broker.URL())
	checkErr(t, e)
	js, e := jetstream.New(connection)
	checkErr(t, e)
	if _, e = js.Stream(ctx, "test_events_v1"); !errors.Is(e, jetstream.ErrStreamNotFound) {
		connection.Close()
		t.Fatal("Setup initialized broker/runtime before a usable profile", e)
	}
	connection.Close()
	profile := command("model", "add", "http://127.0.0.1:1/v1", "fixture-model")
	id := profile["profile_id"].(string)
	command("model", "test", id)
	command("model", "select", id, fmt.Sprint(profile["version"]), fmt.Sprint(status["config_version"]))
	command("chat", "send", "lifecycle-first")
	wait("first recovery completion", func() bool { return stateCount(t, store, "SELECT count(*) FROM run WHERE status='completed'") == 1 })
	broker.Stop()
	wait("outage observed", func() bool {
		raw, _ := os.ReadFile(filepath.Join(data, "logs", "app.jsonl"))
		return bytes.Contains(raw, []byte("BROKER_UNAVAILABLE"))
	})
	command("chat", "send", "lifecycle-offline")
	broker.Start(t)
	wait("reconnect and active work", func() bool { return stateCount(t, store, "SELECT count(*) FROM run WHERE status='completed'") == 2 })
	checkErr(t, child.Process.Signal(syscall.SIGTERM))
	checkErr(t, child.Wait())
	waited = true
	if stderr.Len() != 0 {
		t.Fatal("unclean lifecycle", stderr.String())
	}
	if stateCount(t, store, "SELECT count(*) FROM run WHERE status='cancelled'") != 0 || stateCount(t, store, "SELECT count(*) FROM run_claim WHERE token IS NOT NULL") != 0 || stateCount(t, store, "SELECT count(*) FROM model_generation WHERE attempt_count!=1") != 0 {
		t.Fatal("shutdown cancelled or duplicated accepted work")
	}
	if stateCount(t, store, "SELECT count(*) FROM event_inbox WHERE consumer='runtime_wake_v1'") == 0 {
		t.Fatal("production consumer never committed Inbox")
	}
	log, e := os.ReadFile(filepath.Join(data, "logs", "app.jsonl"))
	checkErr(t, e)
	var stages []string
	for _, line := range bytes.Split(log, []byte{'\n'}) {
		if len(line) == 0 {
			continue
		}
		var row map[string]any
		checkErr(t, json.Unmarshal(line, &row))
		if row["pid"] == float64(child.Process.Pid) {
			if stage, ok := row["stage"].(string); ok {
				stages = append(stages, stage)
			}
		}
	}
	position := -1
	for _, expected := range []string{"storage_ready", "provider_ready", "broker_ready", "tools_ready", "outboxes_started", "recovery_complete", "ingress_open", "ingress_closed"} {
		found := -1
		for i := position + 1; i < len(stages); i++ {
			if stages[i] == expected {
				found = i
				break
			}
		}
		if found < 0 {
			t.Fatal("startup/shutdown order drift", expected, stages)
		}
		position = found
	}
	if path := os.Getenv("ERII_EVENT_EVIDENCE"); path != "" {
		body, _ := json.MarshalIndent(map[string]any{"passed": true, "stages": stages, "checks": []string{"Setup remains available before broker/Tool initialization", "real CLI/provider Fake + NATS + Sidecar handshake", "authoritative startup recovery", "broker outage/reconnect retains active work", "Inbox deduplication", "SIGTERM joins and releases claims without Task cancellation"}}, "", "  ")
		checkErr(t, os.WriteFile(filepath.Join(filepath.Dir(path), "production-lifecycle.json"), append(body, '\n'), 0600))
	}
}
