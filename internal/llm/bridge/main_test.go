package bridge

import (
	"context"
	"os"
	"testing"
)

const nativeBridgeTestArg = "-steward-test-model-bridge"

// Run the production RPC server in a separate process without depending on a
// prebuilt application binary or the compatibility Node launcher.
func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == nativeBridgeTestArg {
		if err := (&Server{}).Serve(context.Background(), os.Stdin, os.Stdout); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func nativeBridgeProcess(t *testing.T) (string, string) {
	t.Helper()
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return path, nativeBridgeTestArg
}
