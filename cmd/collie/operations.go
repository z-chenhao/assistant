package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"strconv"

	"github.com/z-chenhao/collie/internal/app"
	"github.com/z-chenhao/collie/internal/config"
	"github.com/z-chenhao/collie/internal/llm"
	"github.com/z-chenhao/collie/internal/llm/secret"
	"github.com/z-chenhao/collie/internal/maintenance"
)

func runOfflineOperation(ctx context.Context, cfg config.Config, flags *flag.FlagSet, secretsPath string, out io.Writer) (bool, error) {
	var err error
	if flags.Arg(0) != "ops" {
		return false, nil
	}
	if flags.Arg(1) == "activate" {
		if err = maintenance.Activate(cfg.App.DataRoot, flags.Arg(2)); err != nil {
			return true, err
		}
		return true, json.NewEncoder(out).Encode(map[string]bool{"activated": true})
	}
	if flags.Arg(1) == "backup" || flags.Arg(1) == "restore" {
		var secrets llm.SecretStore = secret.Store{}
		if secretsPath != "" {
			secrets = secret.TestSocket{Path: secretsPath}
		}
		key, e := secrets.Get(ctx, flags.Arg(3))
		if e != nil || len(key) != 32 {
			return true, errors.New("backup requires a 32-byte SecretStore key")
		}
		defer func() {
			for i := range key {
				key[i] = 0
			}
		}()
		var manifest maintenance.Manifest
		if flags.Arg(1) == "backup" {
			manifest, err = maintenance.Backup(ctx, cfg.App.DataRoot, flags.Arg(2), key, app.SchemaVersions())
		} else {
			manifest, err = maintenance.Restore(ctx, flags.Arg(2), cfg.App.DataRoot, key, app.SchemaVersions())
		}
		if err != nil {
			return true, err
		}
		return true, json.NewEncoder(out).Encode(manifest)
	}
	return false, nil
}
func runOperations(ctx context.Context, application *app.App, cfg config.Config, flags *flag.FlagSet, out io.Writer) error {
	switch flags.Arg(1) {
	case "health", "report":
		v, e := application.Operations(ctx)
		if e != nil {
			return e
		}
		return json.NewEncoder(out).Encode(v)
	case "trace":
		v, e := application.OperationalTrace(ctx, flags.Arg(2))
		if e != nil {
			return e
		}
		return json.NewEncoder(out).Encode(v)
	case "broker":
		v, e := application.BrokerHealth(ctx, cfg)
		if e != nil {
			return e
		}
		return json.NewEncoder(out).Encode(v)
	case "upgrade-broker":
		v, e := application.UpgradeBroker(ctx, cfg)
		if e != nil {
			return e
		}
		return json.NewEncoder(out).Encode(v)
	case "retry-event":
		n, e := strconv.Atoi(flags.Arg(3))
		if e != nil || n < 0 {
			return errors.New("expected non-negative event attempt count")
		}
		if e = application.RetryEvent(ctx, flags.Arg(2), n); e != nil {
			return e
		}
		return json.NewEncoder(out).Encode(map[string]bool{"requeued": true})
	}
	return nil
}
