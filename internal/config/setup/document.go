package setup

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/z-chenhao/collie/internal/config"
	"github.com/z-chenhao/collie/internal/interaction"
	"github.com/z-chenhao/collie/internal/interaction/web"
	"go.yaml.in/yaml/v3"
)

// Document editing preserves unrelated sections, validates the complete merged
// configuration, and confines atomic replacement to the operator's source file.
func (m *Manager) bootstrap() []byte {
	v := map[string]any{"version": 1, "app": map[string]any{"data_root": m.active.App.DataRoot}}
	if m.active.Web != nil {
		v["web"] = m.active.Web
	}
	if m.active.Feishu != nil {
		v["feishu"] = m.active.Feishu
	}
	for _, id := range kinds {
		if id == "computer" {
			continue
		}
		if value := resource(m.active, id); value != nil {
			v[id] = value
		}
	}
	if m.active.Computer != nil {
		v["computer"] = m.active.Computer
		v["javascript"] = m.active.JavaScript
	}
	raw, _ := yaml.Marshal(v)
	return raw
}

// The parent is pinned with os.Root; neither a symlink nor a non-regular config
// can redirect owner writes. Reads are bounded and never return raw parser errors.
func (m *Manager) read() ([]byte, string, error) {
	root, e := os.OpenRoot(filepath.Dir(m.path))
	if e != nil {
		return nil, "", web.ErrCapabilityUnavailable
	}
	defer root.Close()
	name := filepath.Base(m.path)
	info, e := root.Lstat(name)
	if os.IsNotExist(e) {
		if m.seenFile {
			return nil, "", web.ErrCapabilityConflict
		}
		return m.bootstrap(), "absent", nil
	}
	if e != nil || !info.Mode().IsRegular() {
		return nil, "", web.ErrCapabilityUnavailable
	}
	m.seenFile = true
	f, e := root.Open(name)
	if e != nil {
		return nil, "", web.ErrCapabilityUnavailable
	}
	defer f.Close()
	current, e := f.Stat()
	if e != nil || !os.SameFile(info, current) {
		return nil, "", web.ErrCapabilityConflict
	}
	raw, e := io.ReadAll(io.LimitReader(f, maxConfig+1))
	if e != nil || len(raw) > maxConfig {
		return nil, "", web.ErrCapabilityInvalid
	}
	return raw, digest(raw), nil
}
func (m *Manager) decoded(raw []byte) (config.Config, error) {
	// The running process owns environment precedence; settings do not change it.
	c, e := config.Decode(raw, os.LookupEnv)
	if e != nil || c.ValidateSourcePath(m.path) != nil {
		return c, web.ErrCapabilityInvalid
	}
	if c.Memory != nil && !interaction.ValidCredentialRef(c.Memory.EncryptionKeyRef) || c.Embedding != nil && c.Embedding.APIKeyRef != "" && !interaction.ValidCredentialRef(c.Embedding.APIKeyRef) || c.Computer != nil && !interaction.ValidCredentialRef(c.Computer.Guest.PasswordRef) {
		return c, web.ErrCapabilityInvalid
	}
	return c, nil
}
func parseNode(raw []byte) (*yaml.Node, error) {
	var n yaml.Node
	d := yaml.NewDecoder(strings.NewReader(string(raw)))
	if d.Decode(&n) != nil || len(n.Content) != 1 {
		return nil, web.ErrCapabilityInvalid
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return nil, web.ErrCapabilityInvalid
	}
	return &n, nil
}
func replace(n *yaml.Node, key string, value *yaml.Node) {
	root := n.Content[0]
	for i := 0; i < len(root.Content); i += 2 {
		if root.Content[i].Value == key {
			if value == nil {
				root.Content = append(root.Content[:i], root.Content[i+2:]...)
			} else {
				root.Content[i+1] = value
			}
			return
		}
	}
	if value != nil {
		root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, value)
	}
}
func (m *Manager) candidate(id, revision, text string) ([]byte, config.Config, error) {
	var empty config.Config
	if !known(id) || len(text) > maxEdit {
		return nil, empty, web.ErrCapabilityInvalid
	}
	raw, current, e := m.read()
	if e != nil {
		return nil, empty, e
	}
	if revision != current {
		return nil, empty, web.ErrCapabilityConflict
	}
	n, e := parseNode(raw)
	if e != nil || n.Content[0].Kind != yaml.MappingNode {
		return nil, empty, web.ErrCapabilityInvalid
	}
	var value *yaml.Node
	if strings.TrimSpace(text) != "" {
		edit, e := parseNode([]byte(text))
		if e != nil {
			return nil, empty, e
		}
		value = edit.Content[0]
		if value.Tag == "!!null" {
			value = nil
		}
	}
	if id == "computer" && value != nil {
		if value.Kind != yaml.MappingNode {
			return nil, empty, web.ErrCapabilityInvalid
		}
		found := false
		seen := map[string]bool{}
		for i := 0; i < len(value.Content); i += 2 {
			key := value.Content[i].Value
			if seen[key] || key == "computer" && value.Content[i+1].Tag == "!!null" {
				return nil, empty, web.ErrCapabilityInvalid
			}
			seen[key] = true
			if key != "computer" && key != "javascript" {
				return nil, empty, web.ErrCapabilityInvalid
			}
			found = found || key == "computer"
			replace(n, key, value.Content[i+1])
		}
		if !found {
			return nil, empty, web.ErrCapabilityInvalid
		}
	} else {
		replace(n, id, value)
	}

	raw, e = yaml.Marshal(n)
	if e != nil || len(raw) > maxConfig {
		return nil, empty, web.ErrCapabilityInvalid
	}
	c, e := m.decoded(raw)
	return raw, c, e
}
func (m *Manager) write(raw []byte, revision string) error {
	root, e := os.OpenRoot(filepath.Dir(m.path))
	if e != nil {
		return web.ErrCapabilityUnavailable
	}
	defer root.Close()
	nonce := make([]byte, 12)
	if _, e = rand.Read(nonce); e != nil {
		return web.ErrCapabilityUnavailable
	}
	tmp := ".collie-config-" + hex.EncodeToString(nonce)
	f, e := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return web.ErrCapabilityUnavailable
	}
	defer root.Remove(tmp)
	_, e = f.Write(raw)
	if e == nil {
		e = f.Sync()
	}
	e = errors.Join(e, f.Close())
	if e != nil {
		return web.ErrCapabilityUnavailable
	}
	_, current, e := m.read()
	if e != nil {
		return e
	}
	if current != revision {
		return web.ErrCapabilityConflict
	}
	if e = root.Rename(tmp, filepath.Base(m.path)); e != nil {
		return web.ErrCapabilityUnavailable
	}
	d, e := root.Open(".")
	if e != nil {
		return web.ErrCapabilityUnavailable
	}
	defer d.Close()
	if d.Sync() != nil {
		return web.ErrCapabilityUnavailable
	}
	return nil
}
