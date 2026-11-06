package app

import (
	"context"
	connectedapptools "github.com/z-chenhao/assistant/internal/plugin/connectedapps"
)

func (a *App) refreshConnectedApps(ctx context.Context) error {
	a.connectedAppsRegistryMu.Lock()
	defer a.connectedAppsRegistryMu.Unlock()
	core := a.core.Load()
	if core == nil || core.registry == nil {
		return nil
	}
	definitions, err := connectedapptools.Definitions(ctx, a.connectedApps)
	if err != nil {
		return err
	}
	for i := range definitions {
		definitions[i].Spec.CodeModeOnly = true
	}
	return core.registry.ReplaceGroup(connectedapptools.Prefix, definitions)
}
