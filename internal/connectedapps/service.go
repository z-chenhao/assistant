// Package connectedapps owns the user's app connections. A broker catalogue
// entry is a Connector; only an explicit owner-authorized binding is a Connection.
package connectedapps

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/z-chenhao/assistant/internal/connector/composio"
	"github.com/z-chenhao/assistant/internal/interaction"
	"github.com/z-chenhao/assistant/internal/llm"
)

var ErrInvalid = errors.New("invalid app connection")
var ErrUnavailable = errors.New("app service unavailable; configure or check the Composio project")
var ErrConflict = errors.New("app connections changed; refresh and try again")
var ErrDenied = errors.New("app connection is not active")
var ErrAliasRequired = errors.New("choose a label for the additional account")
var ErrAuthConfigRequired = errors.New("this app requires a custom OAuth configuration in the connection service")
var ErrLinkExpired = errors.New("authorization link expired; reconnect this account")

const AuthorizationTTL = 10 * time.Minute

const DefaultKeyRef = "env:COMPOSIO_API_KEY"

type authorization struct {
	Link    composio.Link
	Created time.Time
}

type Connection struct {
	Public              bool   `json:"public,omitempty"`
	ID                  string `json:"id"`
	Toolkit             string `json:"toolkit"`
	Name                string `json:"name"`
	Alias               string `json:"alias"`
	AccountID           string `json:"account_id"`
	Status              string `json:"status"`
	RevocationConfirmed bool   `json:"revocation_confirmed"`
}
type State struct {
	Revision    int64             `json:"-"`
	KeyRef      string            `json:"key_ref"`
	UserID      string            `json:"user_id"`
	Connections []Connection      `json:"connections"`
	AuthConfigs map[string]string `json:"auth_configs,omitempty"`
}

// Store uses compare-and-swap persistence; no transaction spans provider I/O.
type Store interface {
	LoadApps(context.Context) (State, error)
	SaveApps(context.Context, State, int64) error
}
type Service struct {
	Store   Store
	Secrets llm.SecretStore
	// Client is a protocol-peer injection. Production only supplies Secrets.
	Client  composio.Client
	Changed func(context.Context) error
	// Serializes account lifecycle and dispatch. Once Disconnect returns, an
	// accepted invocation that has not dispatched cannot use the old binding.
	mu      sync.RWMutex
	pending map[string]authorization
}
type View struct {
	State            State
	Catalog          composio.Catalog
	Suggested        bool
	Unavailable      bool
	Search, Category string
	Categories       []composio.Category
}

func (s *Service) client(state State) composio.Client {
	c := s.Client
	c.Secrets, c.KeyRef = s.Secrets, state.KeyRef
	return c
}
func (s *Service) State(ctx context.Context) (State, error) {
	state, err := s.Store.LoadApps(ctx)
	if err != nil || state.KeyRef != "" || s.Secrets == nil {
		return state, err
	}
	// A deployment may supply the conventional environment reference without a
	// separate setup write. Never replace an explicitly configured reference.
	key, secretErr := s.Secrets.Get(ctx, DefaultKeyRef)
	if secretErr == nil && len(key) > 0 {
		state.KeyRef = DefaultKeyRef
	}
	clear(key)
	return state, nil
}
func (s *Service) View(ctx context.Context, search, category, cursor string) (View, error) {
	v := View{Search: search, Category: category}
	if len(search) > 200 || len(category) > 100 || len(cursor) > 4096 {
		return v, ErrInvalid
	}
	var err error
	v.State, err = s.State(ctx)
	if err != nil {
		return v, err
	}
	if v.State.KeyRef == "" {
		v.Suggested = true
		v.Catalog.Items = Suggestions(search, category)
		v.Categories = categories(Suggestions("", ""))
		return v, nil
	}
	v.Catalog, err = s.client(v.State).Catalog(ctx, search, category, cursor)
	if err != nil {
		v.Unavailable = true
		return v, nil
	}
	v.Categories = categories(v.Catalog.Items)
	if category != "" {
		found := false
		for _, c := range v.Categories {
			if c.ID == category {
				found = true
			}
		}
		if !found {
			v.Categories = append(v.Categories, composio.Category{ID: category, Name: category})
		}
	}
	return v, nil
}

func categories(items []composio.Toolkit) []composio.Category {
	var result []composio.Category
	seen := map[string]bool{}
	for _, item := range items {
		for _, c := range item.Meta.Categories {
			if c.ID != "" && !seen[c.ID] {
				result = append(result, c)
				seen[c.ID] = true
			}
		}
	}
	return result
}
func (s *Service) save(ctx context.Context, state State) error {
	return s.Store.SaveApps(ctx, state, state.Revision)
}
func (s *Service) changed(ctx context.Context) error {
	if s.Changed != nil {
		return s.Changed(ctx)
	}
	return nil
}

func (s *Service) Configure(ctx context.Context, ref string, revision int64) error {
	return s.ConfigureProject(ctx, ref, revision, nil)
}
func (s *Service) ConfigureProject(ctx context.Context, ref string, revision int64, configs map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !interaction.ValidCredentialRef(ref) {
		return ErrInvalid
	}
	if len(configs) > 128 {
		return ErrInvalid
	}
	for toolkit, id := range configs {
		if !composio.ValidID(toolkit) || !composio.ValidID(id) {
			return ErrInvalid
		}
	}
	state, err := s.State(ctx)
	if err != nil {
		return err
	}
	if state.Revision != revision {
		return ErrConflict
	}
	if state.KeyRef != ref {
		for _, c := range state.Connections {
			if c.Status != "disconnected" {
				return ErrConflict
			}
		}
	}
	candidate := state
	candidate.KeyRef = ref
	if _, err = s.client(candidate).Catalog(ctx, "", "", ""); err != nil {
		return ErrUnavailable
	}
	for toolkit, id := range configs {
		if _, err = s.client(candidate).AuthConfig(ctx, toolkit, id); err != nil {
			return ErrUnavailable
		}
	}
	state.KeyRef = ref
	if configs != nil {
		state.AuthConfigs = configs
	}
	if err = s.save(ctx, state); err != nil {
		return err
	}
	return s.changed(ctx)
}

// Connect never adopts arbitrary project-wide accounts. The only account that
// can activate is the ID returned by this owner's explicit hosted-link flow.
func (s *Service) Connect(ctx context.Context, toolkit, alias, authConfig string) (composio.Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.connect(ctx, toolkit, alias, authConfig)
}
func (s *Service) connect(ctx context.Context, toolkit, alias, authConfig string) (composio.Link, error) {
	if !composio.ValidID(toolkit) || alias != "" && (!composio.ValidID(alias) || len(alias) > 64) || authConfig != "" && !composio.ValidID(authConfig) {
		return composio.Link{}, ErrInvalid
	}
	state, err := s.State(ctx)
	if err != nil {
		return composio.Link{}, err
	}
	if state.KeyRef == "" {
		return composio.Link{}, ErrUnavailable
	}
	if alias == "" {
		for _, c := range state.Connections {
			if c.Toolkit == toolkit && c.Status == "active" && !c.Public {
				return composio.Link{}, ErrAliasRequired
			}
		}
		for i := len(state.Connections) - 1; i >= 0; i-- {
			c := state.Connections[i]
			if c.Toolkit == toolkit && c.Status == "pending" {
				return s.authorization(state, c.ID)
			}
		}
		alias = "personal"
		for _, c := range state.Connections {
			if c.Toolkit == toolkit && c.Alias == alias {
				alias = "personal_" + uuid.NewString()[:8]
			}
		}
	}
	for _, c := range state.Connections {
		if c.Toolkit == toolkit && c.Alias == alias && c.Status != "disconnected" {
			if c.Status == "pending" {
				return s.authorization(state, c.ID)
			}
			return composio.Link{}, ErrConflict
		}
	}
	client := s.client(state)
	app, err := client.Toolkit(ctx, toolkit)
	if err != nil || app.Slug != toolkit {
		return composio.Link{}, ErrUnavailable
	}
	if app.NoAuth {
		state.Connections = append(state.Connections, Connection{ID: uuid.NewString(), Toolkit: toolkit, Name: app.Name, Alias: alias, Status: "active", Public: true})
		if err = s.save(ctx, state); err != nil {
			return composio.Link{}, err
		}
		return composio.Link{}, s.changed(ctx)
	}
	if authConfig == "" {
		authConfig = state.AuthConfigs[toolkit]
	}
	selection, err := client.FindAuthConfig(ctx, toolkit, authConfig)
	if err != nil {
		return composio.Link{}, ErrUnavailable
	}
	// An explicit empty managed-auth list means the provider cannot supply
	// hosted credentials. An omitted list remains unknown (e.g. scoped peers).
	authConfig = selection.ID
	if selection.Readable && authConfig == "" && app.ManagedAuth != nil && len(app.ManagedAuth) == 0 {
		return composio.Link{}, ErrAuthConfigRequired
	}
	link, err := client.Connect(ctx, state.UserID, toolkit, alias, authConfig)
	if err != nil {
		return composio.Link{}, ErrUnavailable
	}
	link.ConnectionID = uuid.NewString()
	link.ExpiresAt = time.Now().Add(AuthorizationTTL)
	state.Connections = append(state.Connections, Connection{ID: link.ConnectionID, Toolkit: toolkit, Name: app.Name, Alias: alias, AccountID: link.AccountID, Status: "pending"})
	if err = s.save(ctx, state); err != nil {
		return composio.Link{}, err
	}
	if s.pending == nil {
		s.pending = map[string]authorization{}
	}
	for id, p := range s.pending {
		if time.Since(p.Created) >= AuthorizationTTL {
			delete(s.pending, id)
		}
	}
	s.pending[link.ConnectionID] = authorization{Link: link, Created: time.Now()}
	return link, nil
}

func (s *Service) authorization(state State, id string) (composio.Link, error) {
	c := find(&state, id)
	if c == nil || c.Status != "pending" {
		return composio.Link{}, ErrDenied
	}
	p, ok := s.pending[id]
	if !ok || time.Since(p.Created) >= AuthorizationTTL {
		return composio.Link{}, ErrLinkExpired
	}
	return p.Link, nil
}
func (s *Service) Continue(ctx context.Context, id string) (composio.Link, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	state, err := s.State(ctx)
	if err != nil {
		return composio.Link{}, err
	}
	return s.authorization(state, id)
}
func (s *Service) Reconnect(ctx context.Context, id string) (composio.Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.State(ctx)
	if err != nil {
		return composio.Link{}, err
	}
	c := find(&state, id)
	if c == nil || c.Status == "active" || c.Status == "disconnected" {
		return composio.Link{}, ErrDenied
	}
	toolkit, alias := c.Toolkit, c.Alias
	if err = s.disconnect(ctx, id); err != nil {
		return composio.Link{}, err
	}
	return s.connect(ctx, toolkit, alias, "")
}

func find(state *State, id string) *Connection {
	for i := range state.Connections {
		if state.Connections[i].ID == id {
			return &state.Connections[i]
		}
	}
	return nil
}
func accountIdentity(a composio.Account, state State, c Connection) bool {
	return a.ID == c.AccountID && a.UserID == state.UserID && a.Toolkit.Slug == c.Toolkit
}
func validAccount(a composio.Account, state State, c Connection) bool {
	return accountIdentity(a, state, c) && !a.Disabled
}
func (s *Service) Check(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.State(ctx)
	if err != nil {
		return err
	}
	c := find(&state, id)
	if c == nil || c.Status == "disconnected" {
		return ErrDenied
	}
	if c.Public {
		app, e := s.client(state).Toolkit(ctx, c.Toolkit)
		if e != nil {
			return ErrUnavailable
		}
		if app.Slug != c.Toolkit || !app.NoAuth {
			return ErrDenied
		}
		return s.changed(ctx)
	}
	a, err := s.client(state).Account(ctx, c.AccountID)
	if err != nil {
		return ErrUnavailable
	}
	if !accountIdentity(a, state, *c) {
		return ErrDenied
	}
	previous := c.Status
	switch {
	case a.Disabled:
		c.Status = "expired"
	case a.Status == "ACTIVE":
		c.Status = "active"
	case a.Status == "INITIALIZING" || a.Status == "INITIATED":
		c.Status = "pending"
	default:
		c.Status = "expired"
	}
	if previous == c.Status {
		return s.changed(ctx)
	}
	if err = s.save(ctx, state); err != nil {
		return err
	}
	if c.Status != "pending" {
		delete(s.pending, id)
	}
	return s.changed(ctx)
}
func (s *Service) Disconnect(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.disconnect(ctx, id)
}
func (s *Service) disconnect(ctx context.Context, id string) error {
	state, err := s.State(ctx)
	if err != nil {
		return err
	}
	c := find(&state, id)
	if c == nil {
		return ErrInvalid
	}
	if c.Status == "disconnected" {
		return nil
	}
	// Local closure is durable before any unreliable remote operation.
	c.Status = "disconnected"
	if err = s.save(ctx, state); err != nil {
		return err
	}
	delete(s.pending, id)
	refreshErr := s.changed(ctx)
	state, err = s.State(ctx)
	if err != nil {
		return err
	}
	c = find(&state, id)
	c.RevocationConfirmed = c.Public
	if !c.Public {
		client := s.client(state)
		account, e := client.Account(ctx, c.AccountID)
		if e == nil && accountIdentity(account, state, *c) {
			c.RevocationConfirmed = client.Disconnect(ctx, c.AccountID)
		}
	}
	if err = s.save(ctx, state); err != nil {
		return err
	}
	return refreshErr
}

func BindingHash(state State, c Connection) string {
	b, _ := json.Marshal(struct {
		Ref, User  string
		Connection Connection
	}{state.KeyRef, state.UserID, Connection{ID: c.ID, Toolkit: c.Toolkit, Alias: c.Alias, AccountID: c.AccountID, Public: c.Public}})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func ToolHash(t composio.Tool) string {
	var schema any
	decoder := json.NewDecoder(bytes.NewReader(t.Input))
	decoder.UseNumber()
	if decoder.Decode(&schema) == nil {
		t.Input, _ = json.Marshal(schema)
	}
	b, _ := json.Marshal(t)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func (s *Service) active(ctx context.Context, id, hash string) (State, Connection, error) {
	state, err := s.State(ctx)
	if err != nil {
		return state, Connection{}, err
	}
	c := find(&state, id)
	if c == nil || c.Status != "active" || BindingHash(state, *c) != hash {
		return state, Connection{}, ErrDenied
	}
	return state, *c, nil
}
func (s *Service) SearchTools(ctx context.Context, id, hash, query, cursor string) (composio.Tools, error) {
	if len(query) > 200 || len(cursor) > 4096 {
		return composio.Tools{}, ErrInvalid
	}
	state, c, err := s.active(ctx, id, hash)
	if err != nil {
		return composio.Tools{}, err
	}
	result, err := s.client(state).SearchTools(ctx, c.Toolkit, query, cursor)
	if err != nil {
		return composio.Tools{}, ErrUnavailable
	}
	filtered := result.Items[:0]
	for _, t := range result.Items {
		if t.Toolkit.Slug == c.Toolkit && (!c.Public || t.NoAuth) && !t.Deprecated && composio.ValidID(t.Slug) && composio.ValidID(t.Version) && t.Version != "latest" && len(t.Input) > 0 {
			filtered = append(filtered, t)
		}
	}
	result.Items = filtered
	return result, nil
}

// Execute is called only after Steward's durable acceptance and approval. The
// explicit account, version and tool descriptor are revalidated at dispatch.
func (s *Service) Execute(ctx context.Context, id, hash, slug, version, descriptor string, args json.RawMessage) (composio.Execution, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(args) > 65536 || !json.Valid(args) || !strings.HasPrefix(strings.TrimSpace(string(args)), "{") {
		return composio.Execution{}, ErrInvalid
	}
	state, c, err := s.active(ctx, id, hash)
	if err != nil {
		return composio.Execution{}, err
	}
	client := s.client(state)
	if !c.Public {
		a, e := client.Account(ctx, c.AccountID)
		if e != nil {
			return composio.Execution{}, ErrUnavailable
		}
		if !validAccount(a, state, c) || a.Status != "ACTIVE" {
			return composio.Execution{}, ErrDenied
		}
	}
	t, err := client.Tool(ctx, slug, version)
	if err != nil {
		return composio.Execution{}, ErrUnavailable
	}
	if t.Slug != slug || t.Version != version || t.Toolkit.Slug != c.Toolkit || (c.Public && !t.NoAuth) || t.Deprecated || ToolHash(t) != descriptor {
		return composio.Execution{}, ErrDenied
	}
	return client.Execute(ctx, state.UserID, c.AccountID, slug, version, args)
}
