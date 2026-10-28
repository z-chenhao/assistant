package bridge

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/jpeg"
	_ "image/png"
	"math"

	"github.com/z-chenhao/erii/internal/llm"
)

type imagePayload struct {
	Data           string `json:"data"`
	MediaType      string `json:"media_type"`
	OriginalWidth  int    `json:"original_width"`
	OriginalHeight int    `json:"original_height"`
	PreviewWidth   int    `json:"preview_width"`
	PreviewHeight  int    `json:"preview_height"`
}

// Image references alone are persisted. Pixels are authorized and decoded in
// the Host at dispatch, then sent only to the frozen model's provider envelope.
// Downsampling keeps all selected pictures inside the existing 1-MiB RPC cap.
func resolveImages(ctx context.Context, r llm.GenerationRequest) (map[string]imagePayload, error) {
	var out map[string]imagePayload
	if !r.Snapshot.Capabilities.ImageInput {
		return nil, nil
	}
	total := 0
	for _, m := range r.Messages {
		for _, ref := range m.ImageRefs {
			if _, ok := out[ref]; ok {
				continue
			}
			if len(out) >= llm.MaxInputImages || r.ResolveImage == nil {
				return nil, llm.ModelIncompatible
			}
			if ctx.Err() != nil {
				return nil, llm.GenerationCancelled
			}
			media, data, err := r.ResolveImage(ctx, ref)
			if err != nil {
				return nil, llm.ModelIncompatible
			}
			payload, err := previewImage(media, data)
			if err != nil {
				return nil, err
			}

			encoded64 := payload.Data
			total += len(encoded64)
			if total > 512<<10 {
				return nil, llm.ModelIncompatible
			}
			if out == nil {
				out = map[string]imagePayload{}
			}
			out[ref] = payload
		}
	}
	return out, nil
}

// Decode and resize one authorized input; selection, deduplication and envelope
// budgeting stay with resolveImages. Invalid files fail before provider dispatch.
func previewImage(media string, data []byte) (imagePayload, error) {
	if len(data) > llm.MaxInputImageBytes || (media != "image/png" && media != "image/jpeg") {
		return imagePayload{}, llm.ModelIncompatible
	}

	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > llm.MaxInputImagePixels {
		return imagePayload{}, llm.ModelIncompatible
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return imagePayload{}, llm.ModelIncompatible
	}
	size := 640
	var encoded bytes.Buffer
	for {
		scale := math.Min(1, float64(size)/float64(max(cfg.Width, cfg.Height)))
		w, h := max(1, int(float64(cfg.Width)*scale)), max(1, int(float64(cfg.Height)*scale))
		small := image.NewRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				small.Set(x, y, img.At(img.Bounds().Min.X+int(float64(x)/scale), img.Bounds().Min.Y+int(float64(y)/scale)))
			}
		}
		encoded.Reset()
		if jpeg.Encode(&encoded, small, &jpeg.Options{Quality: 60}) != nil {
			return imagePayload{}, llm.ModelIncompatible
		}
		if encoded.Len() <= 48<<10 {
			break
		}
		size /= 2
		if size < 80 {
			return imagePayload{}, llm.ModelIncompatible
		}
	}
	scale := math.Min(1, float64(size)/float64(max(cfg.Width, cfg.Height)))
	return imagePayload{Data: base64.StdEncoding.EncodeToString(encoded.Bytes()), MediaType: "image/jpeg", OriginalWidth: cfg.Width, OriginalHeight: cfg.Height, PreviewWidth: max(1, int(float64(cfg.Width)*scale)), PreviewHeight: max(1, int(float64(cfg.Height)*scale))}, nil
}
