package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	processsandbox "github.com/z-chenhao/erii/internal/computer/sandbox"
)

// BenchmarkWorkspaceStandardSeed is an explicit profiling experiment. It does
// not enable cache reuse in the product. Both arms use the real sandbox and a
// new writable cache per invocation; only trusted std outputs seed one arm.
// Run with -benchtime=5x and ERII_WORKSPACE_PROFILE_DIR to retain paired JSON.
func BenchmarkWorkspaceStandardSeed(b *testing.B) {
	if folder := os.Getenv("ERII_WORKSPACE_PROFILE_DIR"); folder != "" {
		for _, fixture := range []string{"coding-v1", "engineering-v1"} {
			if err := os.Remove(filepath.Join(folder, fixture+".json")); err != nil && !os.IsNotExist(err) {
				b.Fatal(err)
			}
		}
	}
	ctx := context.Background()
	seed, err := filepath.EvalSymlinks(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	environment, err := platformSandbox(ctx, seed, seed, runtime.GOROOT())
	if err != nil {
		b.Fatal(err)
	}
	build, err := processsandbox.Command(ctx, filepath.Join(runtime.GOROOT(), "bin/go"),
		[]string{"build", "-race", "testing", "encoding/json"}, []string{runtime.GOROOT()}, seed, seed)
	if err != nil {
		b.Fatal(err)
	}
	build.Env, build.Dir = environment.Env, seed
	started := time.Now()
	if output, err := build.CombinedOutput(); err != nil {
		b.Fatalf("trusted standard-library preparation: %v\n%s", err, output)
	}
	setupSeconds := time.Since(started).Seconds()
	for _, fixture := range []string{"coding-v1", "engineering-v1"} {
		b.Run(fixture, func(b *testing.B) {
			type row struct {
				Pair     int     `json:"pair"`
				Mode     string  `json:"mode"`
				Wall     float64 `json:"wall_seconds"`
				CPU      float64 `json:"cpu_seconds"`
				Copy     float64 `json:"cache_copy_seconds"`
				ExitCode int     `json:"exit_code"`
			}
			var rows []row
			for pair := 1; pair <= b.N; pair++ {
				modes := []string{"cold", "seeded-private-copy"}
				if pair%2 == 0 {
					modes[0], modes[1] = modes[1], modes[0]
				}
				exits := map[string]int{}
				for _, mode := range modes {
					dir, err := filepath.EvalSymlinks(b.TempDir())
					if err != nil {
						b.Fatal(err)
					}
					repo := filepath.Join(dir, "repo")
					if err := os.CopyFS(repo, os.DirFS(filepath.Join("..", "..", "..", "evals", "fixtures", fixture))); err != nil {
						b.Fatal(err)
					}
					copySeconds := 0.0
					if mode != "cold" {
						started := time.Now()
						if err := os.CopyFS(filepath.Join(dir, "cache"), os.DirFS(filepath.Join(seed, "cache"))); err != nil {
							b.Fatal(err)
						}
						copySeconds = time.Since(started).Seconds()
					}
					command, err := platformSandbox(ctx, dir, repo, runtime.GOROOT())
					if err != nil {
						b.Fatal(err)
					}
					started := time.Now()
					output, err := command.CombinedOutput()
					wall := time.Since(started).Seconds()
					if err != nil {
						if _, ok := err.(*exec.ExitError); !ok {
							b.Fatalf("sandbox launch: %v\n%s", err, output)
						}
					}
					code := command.ProcessState.ExitCode()
					// Both locked fixtures contain deliberate user-requested
					// regressions. A launch/compiler failure is not equivalent.
					if code != 1 {
						b.Fatalf("fixture outcome changed (%s/%s): %d\n%s", fixture, mode, code, output)
					}
					if fixture == "engineering-v1" && !bytes.Contains(output, []byte("late result from grandchild")) {
						b.Fatalf("engineering failure was not the locked regression: %s", output)
					}
					if fixture == "coding-v1" && !bytes.Contains(output, []byte("flag provided but not defined: -status")) {
						b.Fatalf("coding failure was not the locked regression: %s", output)
					}
					exits[mode] = code
					rows = append(rows, row{pair, mode, wall, (command.ProcessState.UserTime() + command.ProcessState.SystemTime()).Seconds(), copySeconds, code})
					if err := os.RemoveAll(dir); err != nil {
						b.Fatal(err)
					}
				}
				if exits["cold"] != exits["seeded-private-copy"] {
					b.Fatal("cache seed changed fixture outcome")
				}
			}
			report := map[string]any{"fixture": fixture, "samples": rows, "seed_setup_seconds": setupSeconds,
				"go_version": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH,
				"seed_packages": []string{"testing", "encoding/json"},
				"scope":         "isolated compiler experiment; not a full CI comparison or a product cache policy",
				"cpu_scope":     "sandbox process wait usage; unreaped/namespaced descendants may be excluded"}
			if folder := os.Getenv("ERII_WORKSPACE_PROFILE_DIR"); folder != "" {
				if err := os.MkdirAll(folder, 0700); err != nil {
					b.Fatal(err)
				}
				raw, err := json.MarshalIndent(report, "", "  ")
				if err != nil {
					b.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(folder, fixture+".json"), append(raw, '\n'), 0600); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
