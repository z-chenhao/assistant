package ocr

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/z-chenhao/erii/internal/computer/sandbox"
	"github.com/z-chenhao/erii/internal/media"
)

func (e *Engine) Extract(ctx context.Context, kind string, data []byte) (Document, error) {
	if err := ctx.Err(); err != nil {
		return Document{}, err
	}
	if err := e.verify(ctx); err != nil {
		return Document{}, err
	}
	img, err := media.DecodeImage(kind, data)
	if err != nil {
		return Document{}, err
	}
	width, height := img.Bounds().Dx(), img.Bounds().Dy()
	root, err := os.MkdirTemp("", "erii-ocr-")
	if err != nil {
		return Document{}, ErrUnavailable
	}
	defer os.RemoveAll(root)
	directory := filepath.Join(root, "data")
	if os.Mkdir(directory, 0700) != nil {
		return Document{}, ErrUnavailable
	}
	for _, lang := range e.languages {
		if err := ctx.Err(); err != nil {
			return Document{}, err
		}
		path := filepath.Join(directory, lang.Name+".traineddata")
		if err := copyLanguage(lang, path); err != nil {
			return Document{}, err
		}
	}
	readRoots := []string{}
	if strings.HasPrefix(e.executable, "/opt/homebrew/") {
		readRoots = append(readRoots, "/opt/homebrew")
	}
	args := []string{"stdin", "stdout", "--tessdata-dir", directory, "-l", strings.Join(e.Languages(), "+"), "--oem", "1", "--psm", "3", "-c", "tessedit_create_tsv=1"}
	cmd, err := sandbox.Command(ctx, e.executable, args, readRoots, root, root)
	if err != nil {
		return Document{}, ErrUnavailable
	}
	cmd.Env = cleanEnv()
	cmd.Dir = root
	cmd.Stdin = bytes.NewReader(data)
	output := sandbox.NewOutput(2 << 20)
	cmd.Stdout = output
	_, err = sandbox.Run(cmd, syscall.SIGKILL)
	if ctx.Err() != nil {
		return Document{}, ctx.Err()
	}
	if output.Overflow() {
		return Document{}, media.ErrLimit
	}
	if err != nil {
		return Document{}, ErrUnavailable
	}
	if err = e.verify(ctx); err != nil {
		return Document{}, err
	}
	return parseTSV(output.Bytes(), width, height, e.Languages())
}

func copyLanguage(lang language, path string) error {
	source, err := os.Open(lang.Path)
	if err != nil {
		return ErrChanged
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 64<<20 {
		return ErrChanged
	}
	dest, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0400)
	if err != nil {
		return ErrUnavailable
	}
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(dest, hash), io.LimitReader(source, (64<<20)+1))
	closed := dest.Close()
	if err != nil || closed != nil {
		return ErrUnavailable
	}
	if n > 64<<20 || hex.EncodeToString(hash.Sum(nil)) != lang.Hash {
		return ErrChanged
	}
	return nil
}
