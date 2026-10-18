package main

import (
	"encoding/json"
	"flag"
	"io"
	"os"

	"taskctl/internal/task"
)

func run(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("search", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	status := "" // CLI_STATUS_FLAG
	if err := flags.Parse(args); err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(task.Search(status))
}

func main() {
	if run(os.Args[1:], os.Stdout) != nil {
		os.Exit(1)
	}
}
