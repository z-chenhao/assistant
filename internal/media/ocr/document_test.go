package ocr

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/z-chenhao/collie/internal/tool"
)

const header = "level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n"

func TestOCRSchemaOnlyAcceptsArtifactReference(t *testing.T) {
	if _, err := tool.NewRegistry([]tool.Definition{Definition("fixture")}); err != nil {
		t.Fatal(err)
	}
	if !tool.Validate(Schema, `{"artifactRef":"artifact:opaque"}`) {
		t.Fatal("valid reference rejected")
	}
	for _, args := range []string{`{}`, `{"path":"/etc/passwd"}`, `{"artifactRef":"artifact:opaque","language":"eng"}`, `{"artifactRef":"artifact:opaque","command":"evil"}`, `{"artifactRef":"artifact:opaque","artifactRef":"artifact:another"}`} {
		if tool.Validate(Schema, args) {
			t.Fatal("invalid OCR input accepted", args)
		}
	}
}

func TestTSVValidationAndBoundedPreview(t *testing.T) {
	row := "5\t1\t1\t1\t1\t1\t4\t8\t20\t10\t98.5\thello\n"
	doc, err := parseTSV([]byte(header+row+strings.Replace(row, "\t1\t1\t4", "\t2\t1\t4", 1)), 100, 50, []string{"eng"})
	if err != nil || len(doc.Words) != 2 || doc.Text != "hello\nhello" {
		t.Fatal(doc, err)
	}
	for _, bad := range []string{
		"bad header\n", header + strings.Replace(row, "98.5", "NaN", 1),
		header + strings.Replace(row, "\t4\t8", "\t99\t8", 1), header + "5\tshort\n",
		header + strings.Replace(row, "hello", string([]byte{0xff}), 1),
	} {
		if _, err := parseTSV([]byte(bad), 100, 50, nil); err == nil {
			t.Fatal("malformed TSV accepted")
		}
	}
	doc, err = parseTSV([]byte(header+strings.Repeat(row, 5000)), 100, 50, []string{"eng"})
	if err != nil || !doc.Truncated || len(doc.Words) != 4096 {
		t.Fatal("unbounded words", len(doc.Words), err)
	}
	doc.Text = strings.Repeat("文字", 2000)
	doc.Words[0].Text = strings.Repeat("文字", 100)
	var preview Document
	if err := json.Unmarshal([]byte(doc.Preview("artifact:result")), &preview); err != nil {
		t.Fatal(err)
	}
	if len(preview.Text) > 2048 || len(preview.Words) != 16 || len(preview.Words[0].Text) > 128 || !utf8.ValidString(preview.Text) || !preview.Truncated || preview.DocumentRef != "artifact:result" {
		t.Fatal("preview limits", preview)
	}
	if len(doc.Words) != 4096 || len(doc.Text) != 12000 {
		t.Fatal("preview mutated full document")
	}
}
