package tui

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"golang.org/x/sys/unix"
)

func TestInputByteBoundariesAndRecovery(t *testing.T) {
	for _, value := range []string{strings.Repeat("a", maxLineBytes), strings.Repeat("困", 5461) + "x", strings.Repeat("a", maxLineBytes+1), strings.Repeat("困", 5462)} {
		var events []inputEvent
		err := readInput(context.Background(), iotest.OneByteReader(strings.NewReader(value+"\r\nrecovery\n")), true, false, func(e inputEvent) bool { events = append(events, e); return true })
		if err != nil || len(events) != 2 {
			t.Fatalf("events: %v %v", events, err)
		}
		if len(value) <= maxLineBytes {
			if events[0].kind != inputLine || events[0].text != value {
				t.Fatal("valid input changed")
			}
		} else if events[0].kind != inputRejected || !errors.Is(events[0].err, errLineTooLong) || events[0].text != "" {
			t.Fatal("oversized prefix accepted")
		}
		if events[1].kind != inputLine || events[1].text != "recovery" {
			t.Fatal("reader did not recover")
		}
	}
}

func TestInputReadErrorHasNonnegativeCount(t *testing.T) {
	input := terminalInput{fd: 1 << 30, closed: make(chan struct{})}
	n, err := input.Read(make([]byte, 1))
	if n != 0 || !errors.Is(err, unix.EBADF) {
		t.Fatalf("invalid Reader result: %d %v", n, err)
	}
}

func TestOpenInputPipeCloseAndEOF(t *testing.T) {
	for _, eof := range []bool{false, true} {
		reader, writer, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		input, err := OpenInput(reader)
		if err != nil {
			reader.Close()
			writer.Close()
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { _, err := input.Read(make([]byte, 1)); done <- err }()
		if eof {
			_ = writer.Close()
		} else if err := input.Close(); err != nil {
			t.Fatal(err)
		}
		select {
		case err := <-done:
			if (eof && err != io.EOF) || (!eof && !errors.Is(err, os.ErrClosed)) {
				t.Fatalf("unexpected read: %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("reader survived close/EOF")
		}
		if err := input.Close(); err != nil {
			t.Fatal(err)
		}
		_ = reader.Close()
		_ = writer.Close()
	}
}

func TestInputEditingAndMalformedUTF8(t *testing.T) {
	var events []inputEvent
	input := "困惑\x7f醒\nwrong\x15right\n" + strings.Repeat("x", maxLineBytes+1) + "\x15cleared\n\xffbad\nrecovered\n\x04"
	err := readInput(context.Background(), strings.NewReader(input), true, false, func(e inputEvent) bool { events = append(events, e); return true })
	if err != nil || len(events) != 5 {
		t.Fatalf("events: %v %v", events, err)
	}
	for i, want := range []string{"困醒", "right", "cleared", "", "recovered"} {
		if events[i].text != want {
			t.Fatalf("event %d: %q", i, events[i].text)
		}
	}
	if events[3].kind != inputRejected || !errors.Is(events[3].err, errInvalidUTF8) {
		t.Fatal("malformed input accepted")
	}
}
