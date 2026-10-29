//go:build darwin && cgo

package secret

import (
	"context"
	"github.com/google/uuid"
	"os"
	"testing"
)

func TestKeychainOwnedRoundTrip(t *testing.T) {
	if os.Getenv("COLLIE_TEST_KEYCHAIN") != "1" {
		t.Skip("owned macOS Keychain verification is opt-in; CI uses Fakes/env references")
	}
	ref := "collie-test:" + uuid.NewString()
	ctx := context.Background()
	s := Store{}
	t.Cleanup(func() {
		if err := s.Delete(ctx, ref); err != nil {
			t.Error("owned credential cleanup failed")
		}
	})
	if err := s.Put(ctx, ref, []byte("synthetic-keychain-fixture")); err != nil {
		t.Fatal(err)
	}
	value, err := s.Get(ctx, ref)
	if err != nil || string(value) != "synthetic-keychain-fixture" {
		t.Fatal("keychain readback failed", err)
	}
	if err := s.Put(ctx, ref, []byte("synthetic-updated")); err != nil {
		t.Fatal(err)
	}
	value, err = s.Get(ctx, ref)
	if err != nil || string(value) != "synthetic-updated" {
		t.Fatal("keychain update failed", err)
	}
}
