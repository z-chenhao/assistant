package tui

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"sync"
	"unicode/utf8"

	"github.com/mattn/go-isatty"
	"golang.org/x/sys/unix"
)

const maxLineBytes = 16 << 10

var errLineTooLong = errors.New("line exceeds 16 KiB; enter a shorter line")
var errInvalidUTF8 = errors.New("line is not valid UTF-8")

type inputEvent struct {
	text string
	kind inputKind
	err  error
}
type inputKind uint8

const (
	inputLine inputKind = iota
	inputEcho
	inputRedraw
	inputRejected
)

type terminalInput struct {
	fd         int
	originalFD int
	settings   *unix.Termios
	closed     chan struct{}
	mu         sync.Mutex
	closeOnce  sync.Once
	closeErr   error
}

func (i *terminalInput) echoEnabled() bool {
	return i.settings != nil && i.settings.Lflag&unix.ECHO != 0
}

// OpenInput owns a duplicate, never the caller's descriptor. Terminals
// use noncanonical input so the kernel's much smaller line buffer cannot discard
// an otherwise valid message. Signals and output processing stay enabled.
func OpenInput(file *os.File) (io.ReadCloser, error) {
	fd := int(file.Fd())
	dup, err := unix.Dup(fd)
	if err != nil {
		return nil, errors.New("open terminal input")
	}
	unix.CloseOnExec(dup)
	var settings *unix.Termios
	if isatty.IsTerminal(uintptr(fd)) {
		settings, err = unix.IoctlGetTermios(fd, getTermios)
		if err != nil {
			_ = unix.Close(dup)
			return nil, errors.New("inspect terminal settings")
		}
		mode := *settings
		mode.Lflag &^= unix.ICANON | unix.ECHO
		// A bounded read also handles a signal flushing input between Poll and
		// Read. Do not set O_NONBLOCK: stdin may share stdout's description.
		mode.Cc[unix.VMIN], mode.Cc[unix.VTIME] = 0, 1
		if err = unix.IoctlSetTermios(fd, setTermios, &mode); err != nil {
			_ = unix.Close(dup)
			return nil, errors.New("configure terminal settings")
		}
	}
	return &terminalInput{fd: dup, originalFD: fd, settings: settings, closed: make(chan struct{})}, nil
}

func (i *terminalInput) restore() error {
	var err error
	if i.settings != nil {
		if unix.IoctlSetTermios(i.originalFD, setTermios, i.settings) != nil {
			err = errors.New("restore terminal settings")
		}
	}
	return err
}

func (i *terminalInput) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for {
		i.mu.Lock()
		select {
		case <-i.closed:
			i.mu.Unlock()
			return 0, os.ErrClosed
		default:
		}
		fds := []unix.PollFd{{Fd: int32(i.fd), Events: unix.POLLIN}}
		ready, err := unix.Poll(fds, 100)
		n := 0
		if err == nil && ready > 0 {
			n, err = unix.Read(i.fd, p)
			if n < 0 {
				n = 0
			} // io.Reader never returns a negative byte count.
			if n == 0 && err == nil && (i.settings == nil || fds[0].Revents&unix.POLLHUP != 0) {
				err = io.EOF
			}
		}
		i.mu.Unlock()
		if err == unix.EINTR || err == unix.EAGAIN {
			continue
		}
		if n > 0 || err != nil {
			return n, err
		}
	}
}

func (i *terminalInput) Close() error {
	i.closeOnce.Do(func() {
		close(i.closed)
		// Poll/Read are bounded. Serialize close with them so a reused fd cannot
		// be read accidentally. Session joins the reader before returning.
		i.mu.Lock()
		defer i.mu.Unlock()
		i.closeErr = errors.Join(unix.Close(i.fd), i.restore())
	})
	return i.closeErr
}

// readInput bounds bytes, not runes, and drains a rejected line through its
// delimiter. No prefix of an oversized/malformed line reaches Main or settings.
// Echo/redraw events share the UI's writer rather than racing its other output.
func readInput(ctx context.Context, input io.Reader, interactive, echo bool, send func(inputEvent) bool) error {
	reader := bufio.NewReader(input)
	line := make([]byte, 0, maxLineBytes)
	var rejected error
	skipLF := false
	finish := func() bool {
		if echo && !send(inputEvent{kind: inputEcho, text: "\n"}) {
			return false
		}
		event := inputEvent{kind: inputLine, text: string(line)}
		if rejected != nil {
			event = inputEvent{kind: inputRejected, err: rejected}
		}
		line, rejected = line[:0], nil
		return send(event)
	}
	for ctx.Err() == nil {
		r, size, err := reader.ReadRune()
		if err != nil {
			if errors.Is(err, io.EOF) {
				if len(line) > 0 || rejected != nil {
					finish()
				}
				return nil
			}
			return err
		}
		if skipLF && r == '\n' {
			skipLF = false
			continue
		}
		skipLF = false
		if r == '\n' || r == '\r' {
			skipLF = r == '\r'
			if !finish() {
				return nil
			}
			continue
		}
		if interactive {
			switch r {
			case '\x04':
				if len(line) == 0 && rejected == nil {
					return nil
				}
				if !finish() {
					return nil
				}
				continue
			case '\x15':
				line, rejected = line[:0], nil
				if echo && !send(inputEvent{kind: inputRedraw}) {
					return nil
				}
				continue
			case '\b', '\x7f':
				if len(line) > 0 && rejected == nil {
					_, width := utf8.DecodeLastRune(line)
					line = line[:len(line)-width]
					if echo && !send(inputEvent{kind: inputRedraw, text: string(line)}) {
						return nil
					}
				}
				continue
			}
		}
		if rejected != nil {
			continue
		}
		if r == utf8.RuneError && size == 1 {
			rejected = errInvalidUTF8
			continue
		}
		if len(line)+size > maxLineBytes {
			rejected = errLineTooLong
			continue
		}
		line = utf8.AppendRune(line, r)
		if echo && !send(inputEvent{kind: inputEcho, text: plain(string(r))}) {
			return nil
		}
	}
	return nil
}
