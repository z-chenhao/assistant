package fake

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"io"
	"sync"

	"github.com/z-chenhao/erii/internal/computer"
)

// Driver is an explicit deterministic adapter Fake. It never substitutes for a
// configured real driver. Tests may inject partial failures and navigation.
type Driver struct {
	Mu            sync.Mutex
	Text          string
	Navigation    int
	Geometry      string
	Calls         []string
	Files         map[string][]byte
	FailInventory string
}

func (f *Driver) Inventory(ctx context.Context, i computer.Instance, kind string) ([]computer.Target, error) {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	f.Calls = append(f.Calls, "inventory:"+kind)
	if f.FailInventory == kind {
		return nil, computer.ErrUnavailable
	}
	switch kind {
	case "apps":
		return []computer.Target{{ID: "notes", Kind: "app", Name: "Notes"}}, nil
	case "windows":
		return []computer.Target{{ID: "notes-window", Kind: "window", Name: "Notes"}}, nil
	case "browsers":
		return []computer.Target{{ID: "chromium", Kind: "browser", Name: "Chromium"}}, nil
	case "tabs":
		return []computer.Target{{ID: "fixture-tab", Kind: "tab", Name: "Fixture", Browser: "chromium"}}, nil
	}
	return nil, computer.ErrInvalid
}
func (f *Driver) Observe(ctx context.Context, i computer.Instance, t computer.Target, lease string, options computer.ObserveOptions) (computer.Capture, error) {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	f.Calls = append(f.Calls, "observe:"+t.ID)
	c := computer.Capture{HasAX: options.Accessibility, State: "[0] Notes " + f.Text + " [actions=increment]", Elements: []computer.Element{{Index: 0, Token: "fixture-element", Actions: []string{"increment"}}}, Width: 100, Height: 100, Geometry: f.Geometry, Navigation: fmt.Sprint(f.Navigation)}
	if options.Screenshot {
		var b bytes.Buffer
		png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 100, 100)))
		c.PNG = b.Bytes()
	}
	return c, nil
}
func (f *Driver) Act(ctx context.Context, i computer.Instance, t computer.Target, lease, method string, m map[string]any) (any, error) {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	f.Calls = append(f.Calls, method)
	switch method {
	case "setValue":
		f.Text, _ = m["value"].(string)
	case "typeText", "paste":
		s, _ := m["text"].(string)
		f.Text += s
	case "goto", "reload":
		f.Navigation++
	case "createBrowserTab":
		return computer.Target{ID: "fixture-tab", Kind: "tab", Name: "Fixture", Browser: "chromium"}, nil
	case "playwright.textContent":
		return map[string]any{"value": f.Text}, nil
	case "playwright.count":
		return map[string]any{"value": 1}, nil
	case "playwright.fill":
		f.Text, _ = m["value"].(string)
	case "playwright.download":
		if f.Files == nil {
			f.Files = map[string][]byte{}
		}
		f.Files[m["path"].(string)] = []byte("fixture download")
	}
	return map[string]string{"effect": "confirmed"}, nil
}
func (f *Driver) ReadFile(ctx context.Context, i computer.Instance, path string) (io.ReadCloser, error) {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	v, ok := f.Files[path]
	if !ok {
		return nil, computer.ErrIntegrity
	}
	return io.NopCloser(bytes.NewReader(v)), nil
}
func (f *Driver) WriteFile(ctx context.Context, i computer.Instance, path string, overwrite bool, r io.Reader) error {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	if f.Files == nil {
		f.Files = map[string][]byte{}
	}
	if _, ok := f.Files[path]; ok && !overwrite {
		return computer.ErrIntegrity
	}
	v, e := io.ReadAll(r)
	if e != nil {
		return e
	}
	f.Files[path] = v
	return nil
}
