package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"net"
	"time"

	"github.com/z-chenhao/erii/internal/app"
	"github.com/z-chenhao/erii/internal/config"
	"github.com/z-chenhao/erii/internal/delivery"
	"github.com/z-chenhao/erii/internal/interaction/web"
)

func runWeb(ctx context.Context, application *app.App, cfg config.Config, out io.Writer, foreground func(func(context.Context) error) error) error {
	if cfg.Web == nil {
		return errors.New("web configuration is required")
	}
	bindings, err := application.ChannelBindings(ctx)
	if err != nil {
		return err
	}
	found := false
	for _, b := range bindings {
		if b.Channel == "web" && b.Subject == web.Subject && b.Destination == web.Destination {
			found = true
		}
	}
	if !found {
		if _, err := application.BindChannel(ctx, "web", web.Subject, web.Destination, true, 0); err != nil {
			return err
		}
	}
	return foreground(func(ctx context.Context) error {
		listener, err := net.Listen("tcp", cfg.Web.Listen)
		if err != nil {
			return errors.New("web listener unavailable")
		}
		defer listener.Close()
		if err = json.NewEncoder(out).Encode(map[string]string{"url": "http://" + listener.Addr().String()}); err != nil {
			return err
		}
		return application.ServeWeb(ctx, listener, *cfg.Web)
	})
}

func runChannel(ctx context.Context, application *app.App, flags *flag.FlagSet, in io.Reader, out io.Writer) error {
	var q struct {
		Op          string `json:"op"`
		Channel     string `json:"channel"`
		Subject     string `json:"subject"`
		Destination string `json:"destination"`
		Enabled     bool   `json:"enabled"`
		Version     int64  `json:"version"`
		BindingID   string `json:"binding_id"`
	}
	dec := json.NewDecoder(io.LimitReader(in, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&q); err != nil {
		return errors.New("invalid channel control request")
	}
	return application.ManageChannel(ctx, q.Op, q.Channel, q.Subject, q.Destination, q.Enabled, q.Version, q.BindingID, out)
}

func runDelivery(ctx context.Context, application *app.App, flags *flag.FlagSet, in io.Reader, out io.Writer) error {
	if flags.Arg(1) == "work" {
		if err := application.Deliver(ctx); err != nil {
			return err
		}
	}
	requests, err := application.DeliveryRequests(ctx)
	if err != nil {
		return err
	}
	// The CLI protocol uses RFC 3339 UTC; SQLite keeps UTC microseconds.
	views := make([]struct {
		delivery.Request
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	}, len(requests))
	for i, r := range requests {
		views[i].Request = r
		views[i].CreatedAt = time.UnixMicro(r.CreatedAt).UTC()
		views[i].UpdatedAt = time.UnixMicro(r.UpdatedAt).UTC()
	}
	return json.NewEncoder(out).Encode(views)
}
