// Package media owns bounded content decoding, independent of storage and models.
package media

import (
	"bytes"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
)

const MaxImageBytes = 8 << 20
const MaxImagePixels = 32 << 20

var ErrInvalid = errors.New("invalid image")
var ErrLimit = errors.New("image limit exceeded")

func ImageConfig(kind string, data []byte) (image.Config, error) {
	if len(data) > MaxImageBytes {
		return image.Config{}, ErrLimit
	}
	if kind != "image/png" && kind != "image/jpeg" {
		return image.Config{}, ErrInvalid
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 {
		return image.Config{}, ErrInvalid
	}
	if int64(cfg.Width) > MaxImagePixels/int64(cfg.Height) {
		return image.Config{}, ErrLimit
	}
	return cfg, nil
}

func DecodeImage(kind string, data []byte) (image.Image, error) {
	if _, err := ImageConfig(kind, data); err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, ErrInvalid
	}
	return img, nil
}
