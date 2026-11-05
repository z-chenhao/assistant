package llm_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// Check the compiler's dependency graph, including indirect dependencies, so
// moving an import behind another adapter cannot bypass the domain boundary.
func TestModelDependencyBoundaries(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "list", "-json", ".", "./provider", "./account", "./sqlite")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	raw, err := cmd.Output()
	if err != nil {
		t.Fatalf("list model dependencies: %v\n%s", err, &stderr)
	}
	packages := map[string][]string{}
	root := ""
	decoder := json.NewDecoder(bytes.NewReader(raw))
	for {
		var p struct {
			ImportPath string
			Deps       []string
		}
		if err := decoder.Decode(&p); err == io.EOF {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		packages[p.ImportPath] = p.Deps
		if strings.HasSuffix(p.ImportPath, "/internal/llm") {
			root = p.ImportPath
		}
	}
	if root == "" || len(packages) != 4 {
		t.Fatal("model packages were not inspected", packages)
	}
	for path, deps := range packages {
		for _, dep := range deps {
			switch path {
			case root:
				if strings.HasPrefix(dep, root+"/") {
					t.Errorf("model contracts depend on an adapter: %s", dep)
				}
			case root + "/provider":
				if strings.HasPrefix(dep, strings.TrimSuffix(root, "/llm")+"/") && !strings.HasPrefix(dep, root+"/provider/") {
					t.Errorf("wire protocols depend on Steward domain state: %s", dep)
				}
			case root + "/account", root + "/sqlite":
				if dep == root+"/bridge" || strings.HasPrefix(dep, root+"/bridge/") {
					t.Errorf("%s depends on RPC transport: %s", path, dep)
				}
			}
		}
	}
}
