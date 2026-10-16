package bridge

import (
	"context"
	"time"

	"github.com/z-chenhao/erii/internal/llm"
)

// Connection is an ephemeral Host-to-Bridge Secret RPC envelope. Never persist it.
type Connection struct {
	Endpoint string `json:"endpoint"`
	APIKey   string `json:"api_key,omitempty"`
}
type Resolver func(context.Context, llm.Snapshot) (Connection, error)
type Observer func(context.Context, llm.GenerationRequest, llm.GenerationResult, time.Duration, error) error

// OpenLocal installs trusted resolution before any request can use the client.
func OpenLocal(ctx context.Context, path, script string, resolver Resolver, observer Observer) (*Client, error) {
	c, err := Open(ctx, path, script)
	if err != nil {
		return nil, err
	}
	c.resolver, c.observer = resolver, observer
	return c, nil
}

func (c *Client) CatalogForProfile(ctx context.Context, p llm.Profile) ([]llm.CatalogModel, error) {
	if c.resolver == nil {
		return c.List(ctx)
	}
	hash, err := llm.EndpointHash(p.Endpoint)
	if err != nil {
		return nil, llm.ModelIncompatible
	}
	connection, err := c.resolver(ctx, llm.Snapshot{ProviderKind: p.ProviderKind, WireAPI: p.WireAPI, EndpointHash: hash, AuthConnectionID: p.AuthConnectionID})
	if err != nil {
		return nil, err
	}
	return c.CatalogAt(ctx, connection)
}

func (c *Client) CatalogAt(ctx context.Context, connection Connection) ([]llm.CatalogModel, error) {
	var models []llm.CatalogModel
	err := c.call(ctx, "models.list", struct {
		Connection Connection `json:"connection"`
	}{connection}, &models)
	return models, err
}

// Health checks the RPC process, independently of optional catalog support.
func (c *Client) Health(ctx context.Context) error {
	if c.resolver == nil {
		_, err := c.List(ctx)
		return err
	}
	var hello struct {
		ProtocolVersion int `json:"protocol_version"`
	}
	if err := c.call(ctx, "bridge.initialize", map[string]int{"protocol_version": ProtocolVersion}, &hello); err != nil {
		return err
	}
	if hello.ProtocolVersion != ProtocolVersion {
		return llm.ModelIncompatible
	}
	return nil
}
