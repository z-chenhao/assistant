package observability

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestConcurrentRotationBoundsAndJSONRecords(t *testing.T) {
	dir := t.TempDir()
	var wg sync.WaitGroup
	for worker := range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w, e := newWriter(dir, "app.jsonl", 1024, 2)
			if e != nil {
				t.Error(e)
				return
			}
			for seq := range 80 {
				v := []byte(fmt.Sprintf("{\"worker\":%d,\"sequence\":%d}\n", worker, seq))
				if _, e = w.Write(v); e != nil {
					t.Error(e)
					return
				}
			}
		}()
	}
	wg.Wait()
	files, e := filepath.Glob(filepath.Join(dir, "app.jsonl*"))
	if e != nil {
		t.Fatal(e)
	}
	count := 0
	for _, name := range files {
		if filepath.Ext(name) == ".lock" {
			continue
		}
		count++
		info, e := os.Stat(name)
		if e != nil || info.Size() > 1024 {
			t.Fatal("unbounded file", name, e)
		}
		f, e := os.Open(name)
		if e != nil {
			t.Fatal(e)
		}
		scan := bufio.NewScanner(f)
		for scan.Scan() {
			var v map[string]int
			if json.Unmarshal(scan.Bytes(), &v) != nil || len(v) != 2 {
				t.Fatal("interleaved JSON")
			}
		}
		if e = scan.Err(); e != nil {
			t.Fatal(e)
		}
		f.Close()
	}
	if count != 3 {
		t.Fatal("rotation history missing", count)
	}
}
