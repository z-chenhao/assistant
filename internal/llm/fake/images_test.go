package fake

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"testing"

	"github.com/z-chenhao/collie/internal/llm"
)

func TestImageFakeDecodesActualPixelsAndRejectsMissingOrInvalidEnvelope(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for y := range 16 {
		for x := range 16 {
			img.Set(x, y, color.RGBA{B: 255, A: 255})
		}
	}
	var buffer bytes.Buffer
	if err := jpeg.Encode(&buffer, img, nil); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{"images": map[string]any{"allowed": map[string]string{"data": base64.StdEncoding.EncodeToString(buffer.Bytes())}}})
	request := llm.GenerationRequest{Messages: []llm.Message{{ImageRefs: []string{"allowed"}}}}
	if got, err := imageInput(raw, request); err != nil || got != "blue" {
		t.Fatalf("pixels: %s %v", got, err)
	}
	for _, invalid := range []string{`{}`, `{"images":{"allowed":{"data":"bad"}}}`, `{"images":{"allowed":{"data":"bm90IGFuIGltYWdl"}}}`} {
		if _, err := imageInput(json.RawMessage(invalid), request); err != llm.InvalidProviderResponse {
			t.Fatalf("invalid envelope: %v", err)
		}
	}
}
