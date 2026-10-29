package fake

import (
	"context"
	"database/sql"
	"io"
	"path/filepath"
	"testing"

	"github.com/z-chenhao/collie/internal/tool"
)

func TestReceiptVisibilityRequiresFreshClient(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	producer := Definition(root, "query_before_retry", nil)
	req := tool.Request{IdempotencyKey: "accepted-effect", Arguments: `{"target":"R1","text":"binary receipt","behavior":"binary_receipt_after_restart"}`}
	if out := producer.Invoke(ctx, req); !out.Ambiguous {
		t.Fatal(out)
	}
	for i := 0; i < 20; i++ {
		if evidence := producer.Query(ctx, req); evidence.State != "unknown" {
			t.Fatal("producing client prematurely exposed receipt", evidence)
		}
	}
	consumer := Definition(root, "query_before_retry", nil)
	evidence := consumer.Query(ctx, req)
	if evidence.State != "succeeded" || evidence.Result.Output == nil {
		t.Fatal(evidence)
	}
	data, err := io.ReadAll(evidence.Result.Output.Reader)
	if closer, ok := evidence.Result.Output.Reader.(io.Closer); ok {
		defer closer.Close()
	}
	if err != nil || string(data) != "Effect recorded for R1: binary receipt" {
		t.Fatal(string(data), err)
	}
	db, err := sql.Open("sqlite", filepath.Join(root, "effect-ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err = db.QueryRow(`SELECT count(*) FROM effects`).Scan(&count); err != nil || count != 1 {
		t.Fatal("Query dispatched duplicate effect", count, err)
	}
}
