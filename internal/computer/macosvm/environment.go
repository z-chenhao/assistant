package macosvm

import (
	"bytes"
	"context"
	"time"

	"github.com/z-chenhao/erii/internal/computer"
	"github.com/z-chenhao/erii/internal/computer/environment"
	"github.com/z-chenhao/erii/internal/tool"
)

// ExecuteEnvironment runs only in the authenticated, epoch-bound Guest. Its
// working files are visible to that same Computer's apps and browser.
func (p *Provider) ExecuteEnvironment(ctx context.Context, instance computer.Instance, name, arguments string) tool.Result {
	g, ip, err := p.useGuest(ctx, instance)
	if err != nil {
		return tool.Failure("PROVIDER_UNAVAILABLE")
	}
	root := "/Users/" + g.Config.User + "/EriiWorkspace"
	request, err := environment.Request(root, name, arguments)
	if err != nil {
		return tool.Failure("INVALID_ARGUMENT")
	}
	// No credentials or model text are interpolated into shell source.
	output, err := g.commandInputFor(ctx, ip, "/usr/bin/python3 -I -c "+quote(environment.Source), false, bytes.NewReader(request), 300*time.Second)
	if err != nil {
		if name != "builtin__read" {
			return tool.Failure("UNKNOWN_SIDE_EFFECT")
		}
		return tool.Failure("PROVIDER_UNAVAILABLE")
	}
	return environment.Response([]byte(output))
}
