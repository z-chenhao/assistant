package app

import (
	"context"
	"strings"

	"github.com/z-chenhao/assistant/internal/config"
	"github.com/z-chenhao/assistant/internal/evidence"
	"github.com/z-chenhao/assistant/internal/memory"
	"github.com/z-chenhao/assistant/internal/observer/github"
)

// Evidence control uses the normal data-root startup checks without attaching
// Memory maintenance or the embedding index. The Vault is read-only here.
func OpenEvidenceControl(ctx context.Context, cfg config.Config, fixture, secretsPath string) (*App, error) {
	var keyRef string
	if cfg.Memory != nil {
		keyRef = cfg.Memory.EncryptionKeyRef
	}
	cfg.Memory = nil
	cfg.Embedding = nil
	a, err := OpenControl(ctx, cfg, fixture, secretsPath)
	if err == nil {
		a.evidenceVault = &memory.Vault{Secrets: a.control.Secrets, Ref: keyRef}
	}
	return a, err
}

func (a *App) ReadEvidence(ctx context.Context, q evidence.Request) (evidence.Page, error) {
	if strings.HasPrefix(q.Ref, "memory:") {
		if a.evidenceVault == nil {
			return evidence.Page{}, evidence.ErrInvalid
		}
		return a.runtimeRuns().ReadMemoryEvidence(ctx, q, a.memory, *a.evidenceVault)
	}
	if a.evidenceVault != nil && (strings.HasPrefix(q.Ref, "task:") || strings.HasPrefix(q.Ref, "github:")) {
		var cfg *github.Config
		if a.github != nil {
			cfg = &a.github.Config
		}
		return a.runtimeRuns().ReadGitHubLinkedEvidence(ctx, q, cfg)
	}
	return a.runtimeRuns().ReadEvidence(ctx, q)
}
