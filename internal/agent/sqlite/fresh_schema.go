package sqlite

import (
	"crypto/sha256"
	_ "embed"
	"fmt"
)

// This is a compiled form of the first twenty migrations, not another schema
// version. Later migrations still apply normally, including on new databases.
const freshSchemaVersion = 20

//go:embed schema-020.sql
var freshSchemaSQL string

func freshSchemaHeader(scripts []string) string {
	hash := sha256.New()
	for _, script := range scripts {
		fmt.Fprintf(hash, "%d\n", len(script))
		hash.Write([]byte(script))
	}
	return fmt.Sprintf("-- Generated empty schema v%d; migrations SHA-256: %x\n"+
		"-- Update: go test ./internal/agent/sqlite -run '^TestFreshSchemaSnapshotMatchesHistory$' -args -update-state-schema\n", freshSchemaVersion, hash.Sum(nil))
}
