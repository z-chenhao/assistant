package macosvm

import (
	"context"
	"io"
	"os"
)

// Linux supports deterministic contract fixtures, not Apple virtualization.
func copyFile(ctx context.Context, from, to string) error {
	f, err := plainFile(from)
	if err != nil {
		return err
	}
	defer f.Close()
	out, err := os.OpenFile(to, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer out.Close()
	buffer := make([]byte, 1024*1024)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := f.Read(buffer)
		if n > 0 {
			if _, err := out.Write(buffer[:n]); err != nil {
				return err
			}
		}
		if err == io.EOF {
			return out.Sync()
		}
		if err != nil {
			return err
		}
	}
}
