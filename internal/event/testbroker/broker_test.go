package testbroker

import "testing"

func TestFixtureCapacityIsIndependentOfSharedTemporaryFilesystem(t *testing.T) {
	b := New(t)
	for round := range 2 {
		cfg := b.Server.JetStreamConfig()
		if cfg.MaxStore != 2<<30 || cfg.MaxMemory != 64<<20 {
			t.Fatalf("round %d: fixture capacity depends on host resources: store=%d memory=%d", round, cfg.MaxStore, cfg.MaxMemory)
		}
		if round == 0 {
			b.Stop()
			b.Start(t)
		}
	}
}
