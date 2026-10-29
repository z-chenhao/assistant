package fake

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	_ "image/jpeg"

	"github.com/z-chenhao/collie/internal/llm"
)

// Decode the actual Host image envelope in the explicit Fake. This proves
// scoped pixel transport and the behavioral image Probe, never OCR quality.
func imageInput(raw json.RawMessage, request llm.GenerationRequest) (string, error) {
	var wire struct {
		Images map[string]struct{ Data, MediaType string } `json:"images"`
	}
	if json.Unmarshal(raw, &wire) != nil {
		return "", llm.InvalidProviderResponse
	}
	color := ""
	for _, message := range request.Messages {
		for _, ref := range message.ImageRefs {
			payload, ok := wire.Images[ref]
			data, err := base64.StdEncoding.DecodeString(payload.Data)
			if !ok || err != nil {
				return "", llm.InvalidProviderResponse
			}
			img, _, err := image.Decode(bytes.NewReader(data))
			if err != nil {
				return "", llm.InvalidProviderResponse
			}
			r, g, b, _ := img.At(img.Bounds().Dx()/2, img.Bounds().Dy()/2).RGBA()
			color = "red"
			if g > r && g > b {
				color = "green"
			} else if b > r && b > g {
				color = "blue"
			}
		}
	}
	return color, nil
}
