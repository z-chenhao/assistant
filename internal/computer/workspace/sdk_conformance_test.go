package workspace

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	processsandbox "github.com/z-chenhao/erii/internal/computer/sandbox"
)

// This additional CI provisioning gate never targets a developer's installed SDK.
// The original cold-cache workspace/security tests remain enabled independently.
func TestCIInstalledSDKConformance(t *testing.T) {
	if os.Getenv("ERII_CI_PUBLIC_SDK_PROBE") != "1" {
		t.Skip("explicit copied-CI-SDK probe")
	}
	repoRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	sdk := filepath.Join(repoRoot, ".ci-go", "sdk")
	if runtime.GOROOT() != sdk {
		t.Fatal("probe requires the project-owned disposable SDK")
	}
	if _, err := os.Stat(filepath.Join(repoRoot, ".ci-go", "sdk.json")); err != nil {
		t.Fatal("missing provisioned SDK receipt")
	}
	archive := filepath.Join(sdk, "pkg", runtime.GOOS+"_"+runtime.GOARCH+"_race", "testing.a")
	before, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	writeRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	write, err := processsandbox.Command(ctx, "/bin/sh", []string{"-c", `printf BAD > "$1"`, "--", archive}, []string{sdk}, writeRoot, writeRoot)
	if err != nil {
		t.Fatal(err)
	}
	write.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + writeRoot, "TMPDIR=" + writeRoot}
	output, err := write.CombinedOutput()
	if err == nil || !(bytes.Contains(bytes.ToLower(output), []byte("permission denied")) || bytes.Contains(bytes.ToLower(output), []byte("operation not permitted"))) {
		t.Fatalf("SDK mutation was not denied by the sandbox: %v\n%s", err, output)
	}
	after, err := os.ReadFile(archive)
	if err != nil || sha256.Sum256(before) != sha256.Sum256(after) {
		t.Fatal("sandbox changed a public SDK archive")
	}
	missing := filepath.Join(sdk, "pkg", runtime.GOOS+"_"+runtime.GOARCH+"_race", "image", "png.a")
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatal("cache-miss fixture is no longer a missing public SDK package")
	}
	outside, err := os.CreateTemp(filepath.Join(repoRoot, ".ci-go"), "outside-probe-")
	if err != nil {
		t.Fatal(err)
	}
	outsidePath := outside.Name()
	defer os.Remove(outsidePath)
	if _, err := outside.WriteString("CI_HOST_SENTINEL"); err != nil {
		t.Fatal(err)
	}
	if err := outside.Close(); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(writeRoot, "repo")
	if err := os.Mkdir(repo, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module sdk_probe\n\ngo 1.26.2\n"), 0600); err != nil {
		t.Fatal(err)
	}
	source := fmt.Sprintf(`package probe
import("bytes";"image";"image/color";"image/png";"os";"path/filepath";"testing")
func TestPrivateCacheAndSDKMiss(t *testing.T){
 if _,err:=os.ReadFile(%s);err==nil{t.Fatal("host sibling escaped the sandbox")}
 if os.Getenv("GOCACHE")!=%s{t.Fatal("workspace writable cache was shared")}
 if err:=os.WriteFile(filepath.Join(os.Getenv("GOCACHE"),"private-marker"),[]byte("private"),0600);err!=nil{t.Fatal(err)}
 img:=image.NewRGBA(image.Rect(0,0,1,1));img.Set(0,0,color.RGBA{R:255,A:255})
 var out bytes.Buffer;if err:=png.Encode(&out,img);err!=nil{t.Fatal(err)}
 decoded,err:=png.Decode(&out);if err!=nil||decoded.Bounds()!=img.Bounds(){t.Fatal("uncached std package did not execute")}
}
`, strconv.Quote(outsidePath), strconv.Quote(filepath.Join(writeRoot, "cache")))
	if err := os.WriteFile(filepath.Join(repo, "probe_test.go"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	command, err := platformSandbox(ctx, writeRoot, repo, sdk)
	if err != nil {
		t.Fatal(err)
	}
	output, err = command.CombinedOutput()
	if err != nil || strings.Contains(string(output), "(cached)") {
		t.Fatalf("cache-miss program failed or reused a test result: %v\n%s", err, output)
	}
	marker, err := os.ReadFile(filepath.Join(writeRoot, "cache", "private-marker"))
	if err != nil || string(marker) != "private" {
		t.Fatal("private writable cache marker missing")
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatal("cache miss wrote into the shared SDK")
	}
}
