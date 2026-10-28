package sqlite

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"strings"
	"testing"

	"github.com/z-chenhao/erii/internal/agent"
	"github.com/z-chenhao/erii/internal/artifact"
	contextbuilder "github.com/z-chenhao/erii/internal/context"
	"github.com/z-chenhao/erii/internal/conversation"
	"github.com/z-chenhao/erii/internal/llm"
)

func TestUserImagesScopedGenerationAndFrozenRecovery(t *testing.T) {
	for _, mode := range []string{"vision", "text", "legacy"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			s, r, _, a, _ := loopFixture(t)
			if mode != "text" {
				input := modelInput()
				input.Name = "Vision"
				input.Capabilities.ImageInput = true
				profile, err := s.Models().AddProfile(ctx, input)
				if err != nil {
					t.Fatal(err)
				}
				c := control(s)
				proof := passedProbe()
				proof.ImageInput = true
				c.Probe = probeFixture{result: proof}
				if _, err = c.Test(ctx, profile.ProfileID); err != nil {
					t.Fatal(err)
				}
				d, err := s.Diagnostics(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if err = c.Select(ctx, profile.ProfileID, profile.Version, d.ConfigVersion); err != nil {
					t.Fatal(err)
				}
			}
			prior := mustSubmit(t, a, "private", "Unrelated earlier work")
			private := putArtifact(t, s, artifact.Scope{UserID: artifact.LocalUser, AgentID: idAgent(t, s), RunID: prior.RunID}, "PRIVATE_UNGRANTED_IMAGE")
			mustDrain(t, a)
			var pixels bytes.Buffer
			if err := png.Encode(&pixels, image.NewRGBA(image.Rect(0, 0, 12, 7))); err != nil {
				t.Fatal(err)
			}
			ref, err := s.Artifacts().Put(ctx, artifactOwner(t, s), artifact.Input{Name: "photo.png", MediaType: "image/png"}, bytes.NewReader(pixels.Bytes()))
			if err != nil {
				t.Fatal(err)
			}
			receipt, err := r.Submit(ctx, "image", []conversation.ContentPart{{Type: "text", Text: "Inspect this picture."}, {Type: "image", ArtifactRef: ref}})
			if err != nil {
				t.Fatal(err)
			}
			if mode == "legacy" {
				// Construct an unstarted, pre-upgrade fixture. Production never
				// rewrites frozen policies or accepted request representations.
				policy, _ := encoded(contextbuilder.DefaultPolicy())
				if _, err = s.db.Exec(`DELETE FROM run_context_policy WHERE run_id=?`, receipt.RunID); err != nil {
					t.Fatal(err)
				}
				if _, err = s.db.Exec(`INSERT INTO run_context_policy VALUES(?,?)`, receipt.RunID, policy); err != nil {
					t.Fatal(err)
				}
			}
			claim, err := r.Claim(ctx, receipt.RunID)
			if err != nil {
				t.Fatal(err)
			}
			if err = r.Initialize(ctx, claim); err != nil {
				t.Fatal(err)
			}
			run, err := r.Load(ctx, claim)
			if err != nil {
				t.Fatal(err)
			}
			inputs, err := r.Steering(ctx, claim, 0)
			if err != nil {
				t.Fatal(err)
			}
			for _, input := range inputs {
				run.Checkpoint.Messages = append(run.Checkpoint.Messages, agent.AgentMessage{MessageID: input.Message.MessageID, Type: "user", Content: input.Message.Content})
				run.Checkpoint.InputSequence = input.Sequence
			}
			if err = r.Checkpoint(ctx, claim, run.Version, run.Checkpoint); err != nil {
				t.Fatal(err)
			}
			run, err = r.Load(ctx, claim)
			if err != nil {
				t.Fatal(err)
			}
			gen, err := r.Generation(ctx, claim, run)
			if err != nil {
				t.Fatal(err)
			}
			selected := 0
			fallback := false
			for _, m := range gen.Request.Messages {
				selected += len(m.ImageRefs)
				fallback = fallback || strings.Contains(m.Text, "Image pixels are unavailable:")
			}
			want := 1
			if mode == "legacy" {
				want = 0
			}
			if selected != want || (gen.Request.ResolveImage != nil) != (mode == "vision") {
				t.Fatal("wrong image admission", mode, selected, gen.Request.ResolveImage != nil)
			}
			if fallback != (mode == "text") {
				t.Fatal("missing or incorrect modality notice", mode, fallback)
			}
			if mode == "vision" {
				media, data, err := gen.Request.ResolveImage(ctx, ref)
				if err != nil || media != "image/png" || !bytes.Equal(data, pixels.Bytes()) {
					t.Fatal("scoped pixels lost", media, err)
				}
				if _, _, err = gen.Request.ResolveImage(ctx, private); !errors.Is(err, artifact.ErrNotFound) {
					t.Fatal("unselected pixels readable", err)
				}
				forged := gen.Request
				forged.Messages = []llm.Message{{Role: "user", ImageRefs: []string{private}}}
				r.resolveGenerationImages(claim, &forged)
				if _, _, err = forged.ResolveImage(ctx, private); !errors.Is(err, artifact.ErrNotFound) {
					t.Fatal("ungranted pixels readable", err)
				}
			}
			var frozen string
			if err = s.db.QueryRow(`SELECT request_json FROM model_generation WHERE generation_id=?`, gen.Request.GenerationID).Scan(&frozen); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(frozen, "base64") || strings.Contains(frozen, "PRIVATE_UNGRANTED_IMAGE") {
				t.Fatal("bytes/private data in durable request")
			}
			if err = r.Release(ctx, claim); err != nil {
				t.Fatal(err)
			}
			current, err := r.Claim(ctx, receipt.RunID)
			if err != nil {
				t.Fatal(err)
			}
			if gen.Request.ResolveImage != nil {
				if _, _, err = gen.Request.ResolveImage(ctx, ref); !errors.Is(err, agent.ErrClaimLost) {
					t.Fatal("old resolver retained ownership", err)
				}
			}
			run, err = r.Load(ctx, current)
			if err != nil {
				t.Fatal(err)
			}
			replay, err := r.Generation(ctx, current, run)
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := llm.CanonicalJSON(replay.Request)
			if string(raw) != frozen {
				t.Fatal("recovery changed frozen image request")
			}
			if err = s.Models().RecordUsageAnd(ctx, replay.Request, llm.GenerationResult{Text: "Image input checked"}, 0, nil, s.AcceptGeneration); err != nil {
				t.Fatal(err)
			}
			completed, err := r.Generation(ctx, current, run)
			if err != nil || completed.Result == nil || completed.Request.ResolveImage != nil {
				t.Fatal("completed result rereads pixels", err)
			}
		})
	}
}
