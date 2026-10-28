package sandbox

import (
	"io"
	"strings"
	"testing"
)

func TestOutputCapSurvivesStreamCopy(t *testing.T) {
	for _, limit := range []int{0, 20, 100} {
		output := NewOutput(limit)
		// LimitReader has no WriterTo. An embedded bytes.Buffer would let
		// io.Copy use ReaderFrom and bypass Write's capacity check.
		n, err := io.Copy(output, io.LimitReader(strings.NewReader(strings.Repeat("x", 100)), 100))
		if err != nil || n != 100 || len(output.Bytes()) != limit || output.Overflow() != (limit < 100) {
			t.Fatal("stream bypassed output cap", limit, n, len(output.Bytes()), output.Overflow(), err)
		}
		if _, err = output.Write([]byte("more")); err != nil || !output.Overflow() || len(output.Bytes()) != limit {
			t.Fatal("subsequent write exceeded cap")
		}
	}
}
