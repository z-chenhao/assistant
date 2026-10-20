package macosvm

import (
	"context"
	"os"

	"golang.org/x/sys/unix"
)

func copyFile(ctx context.Context, from, to string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f, err := plainFile(from)
	if err != nil {
		return err
	}
	f.Close()
	// APFS clone avoids materializing sparse virtual disks. Never silently fall
	// back to a full 150-GB copy when clone is unsupported on the configured store.
	if err := unix.Clonefile(from, to, unix.CLONE_NOFOLLOW); err != nil {
		return err
	}
	if err := os.Chmod(to, 0600); err != nil {
		return err
	}
	out, err := os.OpenFile(to, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer out.Close()
	return out.Sync()
}
