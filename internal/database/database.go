// Package database defines the local owner's bounded metadata inspection view.
// It accepts named projections and opaque references, never SQL or storage paths.
package database

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

var ErrInvalid = errors.New("invalid database inspection query")
var ErrNotFound = errors.New("database record not found")
var ErrUnavailable = errors.New("database inspection unavailable")

const DefaultLimit = 20
const MaxLimit = 50

func Collections() []string { return []string{"executions", "messages", "memories", "artifacts"} }

type Query struct {
	Collection string `json:"collection"`
	Ref        string `json:"ref,omitempty"`
	Cursor     string `json:"cursor,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}

func (q Query) Validate() error {
	valid := false
	for _, name := range Collections() {
		valid = valid || name == q.Collection
	}
	if !valid || q.Limit < 0 || q.Limit > MaxLimit || len(q.Cursor) > 1024 {
		return ErrInvalid
	}
	if q.Ref != "" && (!ValidRef(q.Ref) || Collection(q.Ref) != q.Collection || q.Cursor != "") {
		return ErrInvalid
	}
	return nil
}

func (q Query) PageSize() int {
	if q.Limit == 0 {
		return DefaultLimit
	}
	return q.Limit
}

func Collection(ref string) string {
	for prefix, kind := range map[string]string{"run:": "executions", "conversation:": "messages", "memory:": "memories", "artifact:": "artifacts"} {
		if strings.HasPrefix(ref, prefix) {
			return kind
		}
	}
	return ""
}

func ValidRef(ref string) bool {
	if len(ref) > 256 {
		return false
	}
	prefix, suffix, ok := strings.Cut(ref, ":")
	if !ok || suffix == "" || Collection(ref) == "" && prefix != "task" {
		return false
	}
	for _, c := range suffix {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == ':') {
			return false
		}
	}
	return true
}

type Field struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
type Link struct {
	Relation string `json:"relation"`
	Ref      string `json:"ref"`
}
type Record struct {
	Ref       string  `json:"ref"`
	Status    string  `json:"status"`
	CreatedAt string  `json:"created_at"`
	Fields    []Field `json:"fields"`
	Links     []Link  `json:"links"`
}
type Page struct {
	Collection string   `json:"collection"`
	Items      []Record `json:"items"`
	Next       string   `json:"next,omitempty"`
}

// Cursor is a position, not an authorization. Its binding prevents accidentally
// continuing another projection or installation's list.
type Cursor struct {
	Binding string `json:"b"`
	At      int64  `json:"a"`
	ID      string `json:"i"`
}

func binding(collection, owner string) string {
	sum := sha256.Sum256([]byte(collection + ":" + owner))
	return hex.EncodeToString(sum[:16])
}
func Next(collection, owner string, at int64, id string) string {
	raw, _ := json.Marshal(Cursor{binding(collection, owner), at, id})
	return base64.RawURLEncoding.EncodeToString(raw)
}
func Position(q Query, owner string) (Cursor, error) {
	var c Cursor
	if err := q.Validate(); err != nil {
		return c, err
	}
	if q.Cursor == "" {
		return c, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(q.Cursor)
	if err != nil {
		return c, ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	var extra any
	if d.Decode(&c) != nil || d.Decode(&extra) != io.EOF || c.Binding != binding(q.Collection, owner) || c.At < 0 || c.ID == "" || len(c.ID) > 256 {
		return Cursor{}, ErrInvalid
	}
	return c, nil
}
