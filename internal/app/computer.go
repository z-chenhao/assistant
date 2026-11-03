package app

import (
	"bytes"
	"context"
	"image/png"

	"github.com/z-chenhao/collie/internal/computer"
)

// ComputerStatus is a read-only operator diagnostic. Lifecycle decisions remain
// inside the accepted Code Mode invocation and are never CLI/model VM commands.
func (a *App) ComputerStatus(ctx context.Context) (computer.Status, error) {
	return a.store.ComputerStatus(ctx)
}

func (a *App) ComputerFrame(ctx context.Context) ([]byte, error) {
	if !a.computerPreviewMu.TryLock() {
		return nil, computer.ErrBusy
	}
	defer a.computerPreviewMu.Unlock()
	if a.computer == nil {
		return nil, computer.ErrUnavailable
	}
	before, err := a.store.ComputerPreviewInstance(ctx)
	if err != nil {
		return nil, err
	}
	frame, err := a.computer.PreviewCapture(ctx, before)
	if err != nil {
		return nil, err
	}
	after, err := a.store.ComputerPreviewInstance(ctx)
	if err != nil || before != after {
		return nil, computer.ErrStale
	}
	config, err := png.DecodeConfig(bytes.NewReader(frame.PNG))
	if err != nil || config.Width < 1 || config.Height < 1 || config.Width > 8192 || config.Height > 8192 || int64(config.Width)*int64(config.Height) > 24000000 {
		return nil, computer.ErrUnavailable
	}
	return frame.PNG, nil
}

func (a *App) ComputerActivity(ctx context.Context) ([]computer.Activity, error) {
	return a.store.Inspector().ComputerActivity(ctx)
}
