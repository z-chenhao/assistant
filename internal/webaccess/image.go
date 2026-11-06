package webaccess

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"math"

	"github.com/z-chenhao/assistant/internal/media"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// Decode before allocation, bound both pixels and bytes, and return a portable
// JPEG. The source image never becomes a host path or a model credential.
func resizeImage(input *Image) (*Image, error) {
	if input == nil || len(input.Data) > base64.StdEncoding.EncodedLen(media.MaxImageBytes) {
		return nil, errors.New("RESOURCE_LIMIT")
	}
	data, err := base64.StdEncoding.DecodeString(input.Data)
	if err != nil {
		return nil, errors.New("INVALID_IMAGE")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width) > media.MaxImagePixels/int64(cfg.Height) {
		return nil, errors.New("INVALID_IMAGE")
	}
	if format != "png" && format != "jpeg" && format != "gif" && format != "webp" {
		return nil, errors.New("INVALID_IMAGE")
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, errors.New("INVALID_IMAGE")
	}
	scale := math.Min(1, math.Min(2000/float64(cfg.Width), 2000/float64(cfg.Height)))
	width, height := max(1, int(float64(cfg.Width)*scale)), max(1, int(float64(cfg.Height)*scale))
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
	var output bytes.Buffer
	if err = jpeg.Encode(&output, dst, &jpeg.Options{Quality: 85}); err != nil {
		return nil, err
	}
	return &Image{Data: base64.StdEncoding.EncodeToString(output.Bytes()), MIMEType: "image/jpeg", Width: width, Height: height}, nil
}

// NormalizeImage validates untrusted image bytes before Artifact persistence.
func NormalizeImage(data, mime string) (*Image, error) {
	return resizeImage(&Image{Data: data, MIMEType: mime})
}
