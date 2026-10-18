package run

import (
	"os"
	"strings"
	"testing"
)

func TestSidecarContract(t *testing.T) {
	b, err := os.ReadFile("../../sidecar/contract.ts")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "task: string") || strings.Contains(string(b), "runId:") {
		t.Fatal("contract")
	}
}
