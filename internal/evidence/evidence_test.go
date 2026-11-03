package evidence

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestMemoryEvidenceReferenceIsOwnerOnly(t *testing.T) {
	const valid = `{"ref":"memory:fact","offset":4,"limit":32,"links_offset":20}`
	if q, err := ParseOwner(valid); err != nil || q.Ref != "memory:fact" || q.Offset != 4 || q.Limit != 32 || q.LinksOffset != 20 {
		t.Fatal(q, err)
	}
	if _, err := Parse(valid); err == nil || ValidRef("memory:fact") {
		t.Fatal("Main gained the owner-only evidence reference")
	}
	for _, raw := range []string{`{"ref":"memory:"}`, `{"ref":"memory:../fact"}`, `{"ref":"memory:fact","user_id":"forged"}`, `{"ref":"memory:fact","offset":null}`} {
		if _, err := ParseOwner(raw); err == nil {
			t.Fatal("invalid owner evidence request accepted", raw)
		}
	}
}

func TestEvidenceRequestAndUTF8Windows(t *testing.T) {
	for _, raw := range []string{`{}`, `{"ref":"current","ref":"task:x"}`, `{"ref":"current","limit":null}`, `{"ref":"memory:x"}`, `{"ref":"task:a/b"}`, `{"ref":"current","scope":"all"}`, `{"ref":"current","limit":3}`, `{"ref":"current","offset":-1}`, `{"ref":"current"} {}`} {
		if _, err := Parse(raw); err == nil {
			t.Fatal("invalid evidence request accepted", raw)
		}
	}
	text := strings.Repeat("汉字🧭", 1000)
	q := Request{Ref: "conversation:opaque", Limit: 4096}
	var joined strings.Builder
	for {
		var page Page
		if err := Window(&page, text, q); err != nil || !utf8.ValidString(page.Text) || len(page.Text) > 4096 {
			t.Fatal(page, err)
		}
		joined.WriteString(page.Text)
		if page.NextOffset == nil {
			break
		}
		if *page.NextOffset <= q.Offset {
			t.Fatal("window made no progress")
		}
		q.Offset = *page.NextOffset
	}
	if joined.String() != text {
		t.Fatal("source bytes changed across UTF-8 windows")
	}
	if err := Window(&Page{}, text, Request{Offset: 1, Limit: 4096}); err == nil {
		t.Fatal("accepted a cursor inside UTF-8 character")
	}
}
