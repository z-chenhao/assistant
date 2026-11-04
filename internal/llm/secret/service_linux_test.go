//go:build linux && cgo

package secret

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/z-chenhao/assistant/internal/llm"
)

func TestServiceRejectsInvalidInputsAndCancellation(t *testing.T) {
	s := Store{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	inputs := []struct {
		ctx context.Context
		ref string
	}{{ctx, "credential:cancelled"}}
	for _, ref := range []string{"", "env:BAD", "nul\x00ref", "line\nref", strings.Repeat("x", 513), string([]byte{255})} {
		inputs = append(inputs, struct {
			ctx context.Context
			ref string
		}{context.Background(), ref})
	}
	for _, input := range inputs {
		if _, err := get(input.ctx, input.ref); err != llm.ErrSecretStore {
			t.Fatal("invalid or cancelled read accepted")
		}
		if err := s.Put(input.ctx, input.ref, []byte("synthetic")); err != llm.ErrSecretStore {
			t.Fatal("invalid or cancelled write accepted")
		}
		if err := remove(input.ctx, input.ref); err != llm.ErrSecretStore {
			t.Fatal("invalid or cancelled delete accepted")
		}
	}
	for _, value := range [][]byte{nil, make([]byte, maxSecretBytes+1)} {
		if err := s.Put(context.Background(), "credential:invalid", value); err != llm.ErrSecretStore {
			t.Fatal("invalid value accepted")
		}
	}
}

// This subprocess reads synthetic bytes only from stdin and emits only the
// requested owned value. No values appear in argv, errors or test diagnostics.
func TestServiceSubprocess(t *testing.T) {
	if os.Getenv("STEWARD_TEST_SECRET_SERVICE_HELPER") != "1" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	s, ref := Store{}, os.Getenv("STEWARD_TEST_SECRET_SERVICE_REF")
	var err error
	switch os.Getenv("STEWARD_TEST_SECRET_SERVICE_OP") {
	case "put":
		var data []byte
		data, err = io.ReadAll(io.LimitReader(os.Stdin, maxSecretBytes+1))
		if err == nil {
			err = s.Put(ctx, ref, data)
		}
		clear(data)
	case "get":
		var data []byte
		data, err = s.Get(ctx, ref)
		if err == nil {
			_, err = os.Stdout.Write(data)
		}
		clear(data)
	case "delete":
		err = s.Delete(ctx, ref)
	default:
		os.Exit(2)
	}
	if err != nil {
		os.Exit(3)
	}
	os.Exit(0)
}

func TestServiceOwnedRoundTrip(t *testing.T) {
	if os.Getenv("STEWARD_TEST_SECRET_SERVICE") != "1" {
		t.Skip("owned native Secret Service verification is opt-in; use the isolated account harness")
	}
	if !serviceReady() {
		t.Fatal("libsecret runtime unavailable")
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal("test executable unavailable")
	}
	ref := "steward-test:" + uuid.NewString()
	other := "credential:" + uuid.NewString()
	ctx := context.Background()
	s := Store{}
	t.Cleanup(func() {
		if err := s.Delete(ctx, ref); err != nil {
			t.Error("owned credential cleanup failed")
		}
		if err := s.Delete(ctx, other); err != nil {
			t.Error("second owned credential cleanup failed")
		}
	})
	if err := s.Put(ctx, other, []byte("synthetic-other-account")); err != nil {
		t.Fatal("second native credential write failed")
	}
	value := []byte{'s', 'y', 'n', '\n', 0, 255}
	if err := s.Put(ctx, ref, value); err != nil {
		t.Fatal("native write failed")
	}
	run := func(op string, input []byte, unavailable bool) ([]byte, error) {
		deadline, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(deadline, executable, "-test.run=^TestServiceSubprocess$")
		cmd.Env = append(os.Environ(), "STEWARD_TEST_SECRET_SERVICE_HELPER=1", "STEWARD_TEST_SECRET_SERVICE_REF="+ref, "STEWARD_TEST_SECRET_SERVICE_OP="+op)
		if unavailable {
			cmd.Env = append(cmd.Env, "DBUS_SESSION_BUS_ADDRESS=unix:path="+t.TempDir()+"/missing")
		}
		cmd.Stdin = bytes.NewReader(input)
		return cmd.Output()
	}
	read, err := run("get", nil, false)
	if err != nil || !bytes.Equal(read, value) {
		t.Fatal("cross-process native read failed")
	}
	updated := []byte("synthetic replacement\n尾行\x00")
	if _, err := run("put", updated, false); err != nil {
		t.Fatal("cross-process native update failed")
	}
	read, err = s.Get(ctx, ref)
	if err != nil || !bytes.Equal(read, updated) {
		t.Fatal("native update readback failed")
	}
	if pid, err := strconv.Atoi(os.Getenv("STEWARD_TEST_SECRET_SERVICE_PID")); err == nil && pid > 1 {
		// The isolated harness owns this daemon. Suspend its replies so the
		// helper must cancel an in-flight native RPC rather than fail fast.
		if err := syscall.Kill(pid, syscall.SIGSTOP); err != nil {
			t.Fatal("owned keyring suspension failed")
		}
		func() {
			defer syscall.Kill(pid, syscall.SIGCONT)
			started := time.Now()
			output, err := run("get", nil, false)
			exit, ok := err.(*exec.ExitError)
			if !ok || exit.ExitCode() != 3 || len(output) != 0 || time.Since(started) > 4*time.Second {
				t.Fatal("in-flight native read did not cancel within its deadline")
			}
		}()
		read, err = s.Get(ctx, ref)
		if err != nil || !bytes.Equal(read, updated) {
			t.Fatal("native access did not recover after cancelled read")
		}
	}
	for _, op := range []string{"get", "put", "delete"} {
		if output, err := run(op, value, true); err == nil || len(output) != 0 {
			t.Fatal("unavailable service accepted an operation or emitted credentials")
		}
	}
	read, err = s.Get(ctx, ref)
	if err != nil || !bytes.Equal(read, updated) {
		t.Fatal("unavailable service changed the owned credential")
	}
	if _, err := run("delete", nil, false); err != nil {
		t.Fatal("cross-process native delete failed")
	}
	if _, err := s.Get(ctx, ref); err != llm.ErrSecretStore {
		t.Fatal("deleted credential remained readable")
	}
	if err := s.Delete(ctx, ref); err != nil {
		t.Fatal("missing credential delete is not idempotent")
	}
	read, err = s.Get(ctx, other)
	if err != nil || string(read) != "synthetic-other-account" {
		t.Fatal("read/update/delete crossed credential identities")
	}
}

func TestServiceLockedDenied(t *testing.T) {
	if os.Getenv("STEWARD_TEST_SECRET_SERVICE_LOCKED") != "1" {
		t.Skip("requires the account harness's isolated locked keyring")
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal("test executable unavailable")
	}
	for _, op := range []string{"get", "put", "delete"} {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		cmd := exec.CommandContext(ctx, executable, "-test.run=^TestServiceSubprocess$")
		cmd.Env = append(os.Environ(), "STEWARD_TEST_SECRET_SERVICE_HELPER=1", "STEWARD_TEST_SECRET_SERVICE_REF=credential:steward-native-locked", "STEWARD_TEST_SECRET_SERVICE_OP="+op)
		cmd.Stdin = strings.NewReader("synthetic-update")
		output, err := cmd.CombinedOutput()
		cancel()
		exit, ok := err.(*exec.ExitError)
		if !ok || exit.ExitCode() != 3 || len(output) != 0 {
			t.Fatal("locked credential operation succeeded, exceeded its bound or exposed diagnostics:", op)
		}
	}
}
