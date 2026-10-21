// Package startup owns the optional single-process desktop deployment. It does
// not change the event protocol, Run ownership, or model selection policy.
package startup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/z-chenhao/erii/internal/maintenance"
	"golang.org/x/sys/unix"
)

// LocalBroker holds the data-root locks until JetStream has fully stopped.
// Its random loopback port is private to this foreground application instance.
type LocalBroker struct {
	server *server.Server
	lock   *os.File
	root   *os.File
}

func OpenLocalBroker(ctx context.Context, root string) (*LocalBroker, error) {
	shared, err := maintenance.Lock(root, false)
	if err != nil {
		return nil, err
	}
	b := &LocalBroker{root: shared}
	fail := func(err error) (*LocalBroker, error) { b.Close(); return nil, err }
	fd, err := unix.Open(filepath.Join(root, ".start.lock"), unix.O_CREAT|unix.O_RDWR|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return fail(errors.New("open local startup lock"))
	}
	b.lock = os.NewFile(uintptr(fd), "local startup lock")
	if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return fail(errors.New("Erii is already running for this data root"))
	}
	dir := filepath.Join(root, "broker")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fail(errors.New("create local broker storage"))
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fail(errors.New("local broker storage must be a real directory"))
	}
	b.server, err = server.NewServer(&server.Options{
		Host: "127.0.0.1", Port: -1, JetStream: true, StoreDir: dir,
		// The normal event stream reserves 1 GiB and its DLQ another 64 MiB.
		NoSigs: true, NoLog: true, JetStreamMaxStore: 2 << 30,
	})
	if err != nil {
		return fail(errors.New("configure local broker"))
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	b.server.Start()
	if !b.server.ReadyForConnections(5 * time.Second) {
		return fail(errors.New("local broker did not become ready"))
	}
	return b, nil
}

func (b *LocalBroker) URL() string { return b.server.ClientURL() }

func (b *LocalBroker) Close() {
	if b.server != nil {
		b.server.Shutdown()
		b.server.WaitForShutdown()
		b.server = nil
	}
	if b.lock != nil {
		b.lock.Close()
		b.lock = nil
	}
	if b.root != nil {
		b.root.Close()
		b.root = nil
	}
}
