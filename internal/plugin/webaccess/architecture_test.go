package webaccess

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

// Inspect transitive compiler dependencies: an adapter hidden behind another
// package must not pull research policy or platform state back into a connector.
func TestWebAccessDependencyBoundaries(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "go", "list", "-json", ".", "../../connector/...", "../../webcontent")
	var stderr bytes.Buffer
	command.Stderr = &stderr
	raw, e := command.Output()
	if e != nil {
		t.Fatalf("list Web Access dependencies: %v\n%s", e, &stderr)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	inspected := 0
	for {
		var pkg struct {
			ImportPath string
			Deps       []string
		}
		if e := decoder.Decode(&pkg); e == io.EOF {
			break
		} else if e != nil {
			t.Fatal(e)
		}
		inspected++
		root := pkg.ImportPath[:strings.Index(pkg.ImportPath, "/internal/")+len("/internal/")]
		for _, dep := range pkg.Deps {
			platform := dep == "database/sql" || dep == root+"agent" || strings.HasPrefix(dep, root+"agent/") || dep == root+"app" || strings.HasPrefix(dep, root+"app/") || dep == root+"llm/bridge"
			policy := strings.HasPrefix(dep, root+"plugin/") || dep == root+"tool" || strings.HasPrefix(dep, root+"tool/")
			if platform || pkg.ImportPath != root+"plugin/webaccess" && policy {
				t.Errorf("%s depends on policy/platform %s", pkg.ImportPath, dep)
			}
			if pkg.ImportPath == root+"webcontent" && (strings.HasPrefix(dep, root+"connector/") || dep == root+"llm" || strings.HasPrefix(dep, root+"llm/")) {
				t.Errorf("content processing depends on a service adapter: %s", dep)
			}
		}
	}
	if inspected == 0 {
		t.Fatal("no Web Access packages inspected")
	}
}
