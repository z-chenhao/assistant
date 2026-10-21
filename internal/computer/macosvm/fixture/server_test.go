package fixture

import (
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func TestShutdownRequiresCompleteSudoInput(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	s, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	client, err := ssh.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(s.Config.Guest.Port)), &ssh.ClientConfig{
		User: "lume", Auth: []ssh.AuthMethod{ssh.Password("fixture-only")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	for _, input := range []string{"", "fixture", "fixture-only\n"} {
		session, err := client.NewSession()
		if err != nil {
			t.Fatal(err)
		}
		stdin, err := session.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := session.Start("/usr/bin/sudo -S -p '' /bin/sh -c 'nohup fixture'"); err != nil {
			t.Fatal(err)
		}
		_, writeErr := io.WriteString(stdin, input)
		stdin.Close()
		err = session.Wait()
		session.Close()
		if input == "fixture-only\n" {
			if writeErr != nil || err != nil {
				t.Fatalf("complete sudo input: write=%v exit=%v", writeErr, err)
			}
		} else if err == nil {
			t.Fatalf("shutdown accepted incomplete sudo input %q", input)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.shuts != 1 {
		t.Fatalf("shutdown count = %d, want 1", s.shuts)
	}
}
