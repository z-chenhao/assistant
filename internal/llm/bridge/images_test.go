package bridge

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/z-chenhao/erii/internal/llm"
	"github.com/z-chenhao/erii/internal/media"
)

func TestImagePreviewBoundsAndEnvelopeSelection(t *testing.T) {
	var pixels bytes.Buffer
	if err := png.Encode(&pixels, image.NewRGBA(image.Rect(0, 0, 2000, 1))); err != nil {
		t.Fatal(err)
	}
	payload, err := previewImage("image/png", pixels.Bytes())
	if err != nil || payload.PreviewWidth != 640 || payload.PreviewHeight != 1 || payload.OriginalWidth != 2000 {
		t.Fatal(payload, err)
	}
	jpeg, err := base64.StdEncoding.DecodeString(payload.Data)
	if err != nil {
		t.Fatal(err)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(jpeg))
	if err != nil || cfg.Width != payload.PreviewWidth || cfg.Height != payload.PreviewHeight {
		t.Fatal(cfg, err)
	}
	oversized := append([]byte(nil), pixels.Bytes()...)
	binary.BigEndian.PutUint32(oversized[16:20], 65536)
	binary.BigEndian.PutUint32(oversized[20:24], 65536)
	binary.BigEndian.PutUint32(oversized[29:33], crc32.ChecksumIEEE(oversized[12:29]))
	for _, tc := range []struct {
		media string
		data  []byte
	}{
		{"image/gif", pixels.Bytes()}, {"image/png", []byte("corrupt")}, {"image/png", make([]byte, media.MaxImageBytes+1)}, {"image/png", oversized},
	} {
		if _, err := previewImage(tc.media, tc.data); err != llm.ModelIncompatible {
			t.Fatal("unsafe image admitted", tc.media, len(tc.data), err)
		}
	}
	reads := 0
	r := llm.GenerationRequest{Snapshot: llm.Snapshot{Capabilities: llm.Capabilities{ImageInput: true}}, ResolveImage: func(context.Context, string) (string, []byte, error) {
		reads++
		return "image/png", pixels.Bytes(), nil
	}, Messages: []llm.Message{{Role: "user", ImageRefs: []string{"same", "same"}}}}
	images, err := resolveImages(context.Background(), r)
	if err != nil || len(images) != 1 || reads != 1 {
		t.Fatal("duplicate pixel reads", images, reads, err)
	}
	r.Messages[0].ImageRefs = nil
	for i := 0; i <= llm.MaxInputImages; i++ {
		r.Messages[0].ImageRefs = append(r.Messages[0].ImageRefs, fmt.Sprint(i))
	}
	if _, err := resolveImages(context.Background(), r); err != llm.ModelIncompatible {
		t.Fatal("unbounded images", err)
	}
}

func TestExplicitScopedImageIsSentToActualPiAndNeverObserver(t *testing.T) {
	ctx := context.Background()
	var pngBytes bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	img.Set(0, 0, color.White)
	png.Encode(&pngBytes, img)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var v map[string]any
		if json.NewDecoder(r.Body).Decode(&v) != nil {
			t.Error("wire decode")
		}
		wire, _ := json.Marshal(v)
		if !strings.Contains(string(wire), "data:image/jpeg;base64,") {
			t.Error("selected image absent from actual provider request", string(wire))
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"seen\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	endpoint := server.URL + "/v1"
	script, _ := filepath.Abs("../../../provider-bridge/bridge.mjs")
	client, err := OpenLocal(ctx, "node", script, func(context.Context, llm.Snapshot) (Connection, error) { return Connection{Endpoint: endpoint}, nil }, func(_ context.Context, r llm.GenerationRequest, _ Connection, _ llm.GenerationResult, _ time.Duration, _ error) error {
		raw, _ := json.Marshal(r)
		if strings.Contains(string(raw), "base64") || strings.Contains(string(raw), "media_type") {
			t.Error("binary entered persisted request")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	hash, _ := llm.EndpointHash(endpoint)
	req := llm.GenerationRequest{GenerationID: "image", RunID: "run", SessionID: "run", Snapshot: llm.Snapshot{SnapshotID: "s", ProviderKind: "openai_compatible", WireAPI: "openai_completions", EndpointHash: hash, ModelID: "fixture-model", ContextWindow: 8192, MaxOutputTokens: 1024, Capabilities: llm.Capabilities{ImageInput: true}, Compatibility: llm.Compatibility{ToolResultFormat: "openai"}}, Messages: []llm.Message{{Role: "user", Text: "selected image", ImageRefs: []string{"artifact:allowed"}}}, ResolveImage: func(_ context.Context, ref string) (string, []byte, error) {
		if ref != "artifact:allowed" {
			t.Fatal("unselected read", ref)
		}
		return "image/png", pngBytes.Bytes(), nil
	}}
	result, err := client.Generate(ctx, req, nil)
	if err != nil || result.Text != "seen" {
		t.Fatal(result, err)
	}
	req.Snapshot.Capabilities.ImageInput = false
	req.ResolveImage = func(context.Context, string) (string, []byte, error) {
		t.Fatal("text-only model read image bytes")
		return "", nil, nil
	}
	images, err := resolveImages(ctx, req)
	if err != nil || images != nil {
		t.Fatal(images, err)
	}
	req.Snapshot.Capabilities.ImageInput = true
	req.ResolveImage = nil
	if _, err := resolveImages(ctx, req); err != llm.ModelIncompatible {
		t.Fatal("missing scope resolver accepted", err)
	}
}
