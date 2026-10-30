package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommandGrammarRejectsBeforeOpeningApplicationState(t *testing.T) {
	root := filepath.Join(t.TempDir(), "untouched")
	t.Setenv("COLLIE_DATA_ROOT", root)
	for _, args := range [][]string{
		{"model", "select", "profile", "1"},
		{"model", "list", "unexpected"},
		{"artifact", "cleanup", "force"},
		{"ops", "trace"},
		{"web", "serve", "unexpected"},
		{"channel", "check", "unexpected"},
		{"delivery", "unknown"},
		{"chat", "send", "one", "two"},
		{"start"},
	} {
		var out, errOut bytes.Buffer
		if err := run(context.Background(), args, strings.NewReader(""), &out, &errOut); err == nil {
			t.Fatal("unsupported grammar admitted", args)
		}
		if _, err := os.Stat(root); !os.IsNotExist(err) {
			t.Fatal("invalid command opened application state", args, err)
		}
	}
}

func TestCommandSelectionKeepsControlAndExecutionKindsDistinct(t *testing.T) {
	for _, test := range []struct {
		args []string
		kind commandKind
	}{
		{[]string{"status"}, commandDiagnostics},
		{[]string{"relay"}, commandRelay},
		{[]string{"model", "list"}, commandModel},
		{[]string{"artifact", "cleanup", "apply"}, commandArtifact},
		{[]string{"chat", "send-parts", "request-key"}, commandChat},
		{[]string{"automation", "work"}, commandAutomation},
		{[]string{"channel", "status"}, commandChannel},
		{[]string{"channel", "check"}, commandChannel},
		{[]string{"ops", "backup", "archive", "credential:key"}, commandOps},
	} {
		options, err := parseOptions(test.args, &bytes.Buffer{})
		if err != nil {
			t.Fatal(err)
		}
		command, err := selectCommand(options.flags, strings.NewReader(""))
		if err != nil || command.kind != test.kind {
			t.Fatal("command classification drift", test.args, command, err)
		}
	}
}
