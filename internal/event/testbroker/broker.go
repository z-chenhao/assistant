// Package testbroker owns an isolated, file-backed JetStream server for tests.
// It is imported only by test files, never the product executable.
package testbroker

import (
	"net"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
)

type Broker struct {
	Server  *server.Server
	options server.Options
}

func New(t testing.TB) *Broker {
	return NewAuthenticated(t, "")
}

func NewAuthenticated(t testing.TB, token string) *Broker {
	t.Helper()
	// Stream MaxBytes reserves capacity without allocating it. Auto-detection
	// would make admission/restart depend on concurrent Go builds consuming the
	// shared CI tmpfs. Fixtures write only their small, bounded event samples.
	b := &Broker{options: server.Options{Host: "127.0.0.1", Port: -1, JetStream: true, StoreDir: t.TempDir(), NoLog: true, NoSigs: true, JetStreamMaxStore: 2 << 30, JetStreamMaxMemory: 64 << 20}}
	b.options.Authorization = token
	b.Start(t)
	t.Cleanup(b.Stop)
	return b
}
func (b *Broker) Start(t testing.TB) {
	t.Helper()
	s, err := server.NewServer(&b.options)
	if err != nil {
		t.Fatal(err)
	}
	b.Server = s
	s.Start()
	if !s.ReadyForConnections(5 * time.Second) {
		b.Stop()
		t.Fatal("isolated broker not ready")
	}
	b.options.Port = s.Addr().(*net.TCPAddr).Port
}
func (b *Broker) URL() string { return b.Server.ClientURL() }
func (b *Broker) Stop() {
	if b.Server != nil {
		b.Server.Shutdown()
		b.Server.WaitForShutdown()
	}
}
