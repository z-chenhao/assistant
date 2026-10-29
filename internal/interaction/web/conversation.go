package web

import (
	"context"
	"strings"

	"github.com/z-chenhao/erii/internal/conversation"
	"github.com/z-chenhao/erii/internal/delivery/ui"
	"github.com/z-chenhao/erii/internal/interaction"
)

func (s *Server) showMessage(ctx context.Context, m conversation.Message, approvals map[string]interaction.Approval) (shownMessage, error) {
	out := shownMessage{Role: m.Role, Sequence: m.Sequence}
	// Match Host-authored notification identity, never strip arbitrary user text
	// by regex or rewrite the durable Conversation/frozen delivery projection.
	if m.Source == "security" {
		id, ok := strings.CutPrefix(m.MessageID, "approval:")
		if !ok {
			id, ok = strings.CutPrefix(m.MessageID, "spend:")
		}
		if a, found := approvals[id]; ok && found {
			v := approvalFor(a)
			out.Approval = &v
			return out, nil
		}
	}
	fallback := ""
	for _, c := range m.Content {
		if c.Type == "text" {
			fallback += c.Text + "\n"
		} else if c.ArtifactRef != "" {
			fallback += c.Type + ": " + c.ArtifactRef + "\n"
		}
	}
	raw, err := s.Core.WebProjection(ctx, m)
	if err != nil {
		return out, err
	}
	out.Projection = shownProjection{HTML: ui.HTML(raw, fallback), Spec: string(raw), Fallback: fallback}
	return out, nil
}
func (s *Server) loadConversation(ctx context.Context, p *page) error {
	approvals, err := s.Core.WebApprovals(ctx)
	if err != nil {
		return err
	}
	indexed := map[string]interaction.Approval{}
	for _, a := range approvals {
		indexed[a.ID] = a
	}
	messages, err := s.Core.Conversation(ctx, p.Cursor)
	if err != nil {
		return err
	}
	p.Before = messages.Before
	shown := map[string]bool{}
	for _, m := range messages.Messages {
		item, err := s.showMessage(ctx, m, indexed)
		if err != nil {
			return err
		}
		if item.Approval != nil {
			shown[item.Approval.ID] = true
		}
		p.Messages = append(p.Messages, item)
		p.Latest = max(p.Latest, m.Sequence)
	}
	for _, a := range approvals {
		if a.Status == "pending" && !shown[a.ID] {
			p.Approvals = append(p.Approvals, approvalFor(a))
		}
	}
	if p.Cursor != 0 {
		latest, err := s.Core.Conversation(ctx, 0)
		if err != nil {
			return err
		}
		for _, m := range latest.Messages {
			p.Latest = max(p.Latest, m.Sequence)
		}
	}
	return nil
}
