package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"time"

	"github.com/z-chenhao/erii/internal/app"
	"github.com/z-chenhao/erii/internal/automation"
	computerfixture "github.com/z-chenhao/erii/internal/computer/macosvm/fixture"
	"github.com/z-chenhao/erii/internal/llm/fake"
	mcpfake "github.com/z-chenhao/erii/internal/tool/mcp/fake"
)

func runTestPeer(ctx context.Context, args []string, in io.Reader, out io.Writer) (bool, error) {
	if len(args) == 2 && args[0] == "-test-computer-server" {
		return true, computerfixture.Serve(ctx, args[1], out)
	}
	if len(args) == 2 && args[0] == "-test-mcp-server" {
		return true, mcpfake.Serve(in, out, args[1])
	}
	if len(args) == 3 && args[0] == "-test-provider-bridge" {
		return true, fake.ServeRecorded(in, out, args[1], args[2])
	}
	if len(args) == 2 && args[0] == "-test-provider-bridge" {
		return true, fake.Serve(in, out, args[1])
	}
	return false, nil
}
func (o commandOptions) configureClock(application *app.App) error {
	if o.testAutomationNow != "" {
		if o.testBridge == "" {
			return errors.New("test-automation-now requires explicit test-bridge")
		}
		now, e := time.Parse(time.RFC3339, o.testAutomationNow)
		if e != nil {
			return automation.ErrInvalid
		}
		application.SetTestAutomationClock(now)
	}
	return nil
}
func (o commandOptions) configureExecution(ctx context.Context, application *app.App, out io.Writer) error {
	if o.testDelivery != "" {
		if o.testBridge == "" {
			return errors.New("test-delivery requires explicit test-bridge")
		}
		if err := application.SetTestDelivery(o.testDelivery); err != nil {
			return err
		}
	}
	if o.testMainHold != "" || len(o.testBarriers) > 0 {
		if o.testBridge == "" {
			return errors.New("test-main-hold requires explicit test-bridge")
		}
		application.SetTestMainBoundary(func(name string) {
			release, held := o.testBarriers[name]
			if name == o.testMainHold {
				release, held = o.testMainRelease, true
			}
			if held {
				_ = json.NewEncoder(out).Encode(struct {
					Boundary string `json:"boundary"`
				}{name})
				if release == "" {
					<-ctx.Done()
				} else {
					ticker := time.NewTicker(10 * time.Millisecond)
					defer ticker.Stop()
					for {
						if _, err := os.Stat(release); err == nil {
							break
						}
						select {
						case <-ctx.Done():
							return
						case <-ticker.C:
						}
					}
				}
			}
		})
	}
	return nil
}
