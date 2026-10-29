package main

import (
	"errors"
	"flag"
	"io"
	"os"
	"slices"

	"github.com/mattn/go-isatty"
)

type commandKind uint8

const (
	commandDiagnostics commandKind = iota
	commandDoctor
	commandTerminal
	commandRelay
	commandModel
	commandChat
	commandArtifact
	commandDelivery
	commandApproval
	commandSkill
	commandMemory
	commandSpend
	commandComputer
	commandWeb
	commandChannel
	commandAutomation
	commandOps
)

type command struct {
	kind  commandKind
	start bool
}
type commandGrammar struct {
	kind  commandKind
	forms map[string][]int
}

// Arity includes the command and operation. The grammar is validated before
// configuration/state is opened; handlers do not independently admit commands.
var commandGrammars = map[string]commandGrammar{
	"model":      {commandModel, map[string][]int{"add": {2, 4, 5}, "discover": {3, 4}, "list": {2}, "cancel": {2}, "accounts": {2}, "catalog": {2, 3}, "login": {2, 3}, "login-chatgpt": {2, 3}, "test": {3}, "refresh": {3}, "logout": {3}, "account-catalog": {3}, "account-add": {4}, "select": {5}}},
	"chat":       {commandChat, map[string][]int{"send": {2, 3}, "send-parts": {2, 3}, "work": {2}, "history": {2}}},
	"artifact":   {commandArtifact, map[string][]int{"save": {4}, "delete": {4}, "open": {3}, "describe": {3}, "cleanup": {2, 3}}},
	"delivery":   {commandDelivery, map[string][]int{"list": {2}, "work": {2}}},
	"approval":   {commandApproval, map[string][]int{"list": {2}, "reconcile": {2}, "approve": {4}, "deny": {4}}},
	"skill":      {commandSkill, map[string][]int{"manage": {2}}},
	"memory":     {commandMemory, map[string][]int{"work": {2}, "status": {2}, "manage": {3}}},
	"spend":      {commandSpend, map[string][]int{"status": {2}}},
	"computer":   {commandComputer, map[string][]int{"status": {2}, "prepare-base": {3}}},
	"web":        {commandWeb, map[string][]int{"serve": {2}}},
	"channel":    {commandChannel, map[string][]int{"manage": {2}}},
	"automation": {commandAutomation, map[string][]int{"manage": {3}, "list": {2}, "tick": {2}, "work": {2}}},
	"ops":        {commandOps, map[string][]int{"health": {2}, "report": {2}, "broker": {2}, "upgrade-broker": {2}, "trace": {3}, "activate": {3}, "backup": {4}, "restore": {4}, "retry-event": {4}}},
}

func selectCommand(flags *flag.FlagSet, in io.Reader) (command, error) {
	if flags.NArg() == 0 {
		if file, ok := in.(*os.File); ok && isatty.IsTerminal(file.Fd()) {
			return command{kind: commandTerminal}, nil
		}
		return command{kind: commandDiagnostics}, nil
	}
	if flags.NArg() == 1 {
		switch flags.Arg(0) {
		case "status":
			return command{kind: commandDiagnostics}, nil
		case "relay":
			return command{kind: commandRelay}, nil
		case "doctor":
			return command{kind: commandDoctor}, nil
		case "tui":
			return command{kind: commandTerminal}, nil
		case "start":
			if file, ok := in.(*os.File); !ok || !isatty.IsTerminal(file.Fd()) {
				return command{}, errors.New("start needs an interactive terminal; use status for diagnostics or web serve for the browser interface")
			}
			return command{kind: commandTerminal, start: true}, nil
		}
	}
	grammar, ok := commandGrammars[flags.Arg(0)]
	if ok && slices.Contains(grammar.forms[flags.Arg(1)], flags.NArg()) {
		if flags.Arg(0) != "artifact" || flags.Arg(1) != "cleanup" || flags.NArg() != 3 || flags.Arg(2) == "apply" {
			return command{kind: grammar.kind}, nil
		}
	}
	return command{}, errors.New("expected status, relay, model add/list/catalog/test/select/cancel, or no command (see -help)")
}
