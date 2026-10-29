package memory

import (
	"bytes"
	"context"
	secrets "github.com/z-chenhao/collie/internal/llm/memory"
	"testing"
)

func TestVaultScopeTamperingAndKeyLossFailClosed(t *testing.T) {
	ctx := context.Background()
	s := secrets.New()
	if e := s.Put(ctx, "key", bytes.Repeat([]byte{3}, 32)); e != nil {
		t.Fatal(e)
	}
	v := Vault{Secrets: s, Ref: "key"}
	sealed, e := v.Seal(ctx, "sensitive record", "alice:record")
	if e != nil {
		t.Fatal(e)
	}
	if plain, e := v.Open(ctx, sealed, "alice:record"); e != nil || plain != "sensitive record" {
		t.Fatal(plain, e)
	}
	if _, e = v.Open(ctx, sealed, "bob:record"); e == nil {
		t.Fatal("cross-scope ciphertext accepted")
	}
	if _, e = v.Open(ctx, sealed+"x", "alice:record"); e == nil {
		t.Fatal("tampered ciphertext accepted")
	}
	if e = s.Delete(ctx, "key"); e != nil {
		t.Fatal(e)
	}
	if _, e = v.Open(ctx, sealed, "alice:record"); e == nil {
		t.Fatal("missing key accepted")
	}
}
func TestExtractorDoesNotTurnNoisyEvidenceIntoFacts(t *testing.T) {
	for _, text := range []string{"My password is a-secret-value", "I have api_key=synthetic-credential", "I wonder whether I have a disorder?", "sk-abcdefghijklmno"} {
		if got := Extract(text, "user"); len(got) != 0 {
			t.Fatal(text, got)
		}
	}
	if got := Extract("User reported: I prefer tea\nI invented an unreported diagnosis", "worker"); len(got) != 1 || got[0] != "I prefer tea" {
		t.Fatal(got)
	}
	if Stable("I might explore independent development") || Stable("I felt tired today") {
		t.Fatal("exploratory/transient report considered stable")
	}
}
