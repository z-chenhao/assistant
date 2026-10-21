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
			if len(out) >= 8 || r.ResolveImage == nil {
				return nil, llm.ModelIncompatible
			}
			media, data, err := r.ResolveImage(ctx, ref)
			if err != nil || len(data) > 8<<20 || (media != "image/png" && media != "image/jpeg") {
				return nil, llm.ModelIncompatible
			}
			cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
			if err != nil || cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > 32<<20 {
				return nil, llm.ModelIncompatible
			}
			img, _, err := image.Decode(bytes.NewReader(data))
			if err != nil {
				return nil, llm.ModelIncompatible
			}
			size := 640
			var encoded bytes.Buffer
			for {
				scale := math.Min(1, float64(size)/float64(max(cfg.Width, cfg.Height)))
				w, h := int(float64(cfg.Width)*scale), int(float64(cfg.Height)*scale)
				if w < 1 || h < 1 {
					return nil, llm.ModelIncompatible
				}
				small := image.NewRGBA(image.Rect(0, 0, w, h))
				for y := 0; y < h; y++ {
					for x := 0; x < w; x++ {
						small.Set(x, y, img.At(img.Bounds().Min.X+int(float64(x)/scale), img.Bounds().Min.Y+int(float64(y)/scale)))
					}
				}
				encoded.Reset()
				if jpeg.Encode(&encoded, small, &jpeg.Options{Quality: 60}) != nil {
					return nil, llm.ModelIncompatible
				}
				if encoded.Len() <= 48<<10 {
					break
				}
				size /= 2
				if size < 80 {
					return nil, llm.ModelIncompatible
				}
			}
			encoded64 := base64.StdEncoding.EncodeToString(encoded.Bytes())
			total += len(encoded64)
			if total > 512<<10 {
				return nil, llm.ModelIncompatible
			}
			if out == nil {
				out = map[string]imagePayload{}
			}
			out[ref] = imagePayload{Data: encoded64, MediaType: "image/jpeg", OriginalWidth: cfg.Width, OriginalHeight: cfg.Height, PreviewWidth: int(float64(cfg.Width) * math.Min(1, float64(size)/float64(max(cfg.Width, cfg.Height)))), PreviewHeight: int(float64(cfg.Height) * math.Min(1, float64(size)/float64(max(cfg.Width, cfg.Height))))}
		}
	}
	return out, nil
}
