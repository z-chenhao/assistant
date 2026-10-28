package agent

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/z-chenhao/erii/internal/conversation"
	"github.com/z-chenhao/erii/internal/llm"
	"github.com/z-chenhao/erii/internal/tool"
)

// RunFailure proves that the Run's terminal failure was committed. Interactive
// surfaces may keep serving; callers can still inspect the provider cause.
type RunFailure struct{ cause error }

func (e *RunFailure) Error() string { return e.cause.Error() }
func (e *RunFailure) Unwrap() error { return e.cause }

type LoopOptions struct {
	Store    RunStore
	Provider llm.LLMProvider
	Tools    tool.Executor
	Sink     EventSink
	// Execution is cancelled for business stop or lost ownership. Shutdown
	// still waits for the current step to persist at its stable boundary.
	Execution context.Context
}

// RunAgentLoop and ContinueAgentLoop share one stateless kernel. Roles supply
// context, tools and a result destination outside this algorithm.
func RunAgentLoop(ctx context.Context, claim RunClaim, options LoopOptions) error {
	return ContinueAgentLoop(ctx, claim, options)
}

func ContinueAgentLoop(ctx context.Context, claim RunClaim, options LoopOptions) error {
	work := context.WithoutCancel(ctx)
	execution := options.Execution
	if execution == nil {
		execution = work
	}
	return (agentLoop{claim: claim, options: options, work: work, execution: execution}).run(ctx)
}

// The runner owns no durable state. Every phase reloads the authoritative Run;
// each phase handler advances exactly one checkpoint or stops at a stable wait.
// Shutdown stops between phases. Business stop/claim loss cancels execution
// while persistence continues through the independent work context.
type agentLoop struct {
	claim     RunClaim
	options   LoopOptions
	work      context.Context
	execution context.Context
}

func (l agentLoop) run(ctx context.Context) error {
	r, err := l.options.Store.Load(l.work, l.claim)
	if err != nil {
		return err
	}
	l.emit(r, "agent_start", "", "")
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		r, err = l.options.Store.Load(l.work, l.claim)
		if err != nil {
			return err
		}
		// Settlement can request consent after a final model result was saved.
		// Hold that result without regenerating it.
		if r.SpendStatus == "pending" {
			return l.options.Store.Wait(l.work, l.claim, r.Version, "resource")
		}
		// Accepted Tool calls must drain and adopt terminal Child results even
		// when monetary authorization is denied at the next model boundary.
		if (r.SpendStatus == "denied" || r.SpendStatus == "invalidated") && r.Checkpoint.Phase != ToolsPhase {
			return l.fail(r, "SPEND_AUTHORIZATION_DENIED")
		}
		var stopped bool
		switch r.Checkpoint.Phase {
		case ModelPhase:
			stopped, err = l.modelTurn(r)
		case ToolsPhase:
			stopped, err = l.toolsTurn(r)
		case ReplyPhase:
			stopped, err = l.replyTurn(r)
		case SettlePhase:
			stopped, err = l.settleTurn(r)
		default:
			return ErrStorage
		}
		if stopped || err != nil {
			return err
		}
	}
}

func (l agentLoop) modelTurn(r Run) (bool, error) {
	cp := r.Checkpoint
	inputs, err := l.options.Store.Steering(l.work, l.claim, cp.InputSequence)
	if errors.Is(err, ErrConflict) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if len(inputs) > 0 {
		for _, input := range inputs {
			cp.Messages = append(cp.Messages, AgentMessage{MessageID: input.Message.MessageID, Type: "user", Content: input.Message.Content})
			cp.InputSequence = input.Sequence
		}
		if err = l.options.Store.Checkpoint(l.work, l.claim, r.Version, cp); err != nil {
			return false, err
		}
		return false, nil
	}
	// Context preparation may retrieve durable evidence. Business stop or
	// lost ownership must cancel those provider reads before model admission.
	generation, err := l.options.Store.Generation(l.execution, l.claim, r)
	if err != nil {
		if errors.Is(err, llm.ContextLengthExceeded) {
			if retry, e := l.retryContext(r); retry {
				if e != nil {
					return false, e
				}
				return false, nil
			}
		}
		if errors.Is(err, ErrResourceLimit) {
			return false, l.fail(r, "RESOURCE_LIMIT")
		}
		var providerError llm.ProviderError
		if errors.As(err, &providerError) {
			return false, l.fail(r, providerError.Error())
		}
		return false, err
	}
	if generation.Waiting {
		if err := l.options.Store.Wait(l.work, l.claim, r.Version, "resource"); err != nil {
			return false, err
		}
		return true, nil
	}
	if generation.Denied {
		return false, l.fail(r, "SPEND_AUTHORIZATION_DENIED")
	}
	if generation.ErrorCode != "" {
		if generation.ErrorCode == llm.ContextLengthExceeded.Error() {
			if retry, e := l.retryContext(r); retry {
				if e != nil {
					return false, e
				}
				return false, nil
			}
		}
		return false, l.fail(r, generation.ErrorCode)
	}
	result := generation.Result
	if result == nil {
		visible := generation.Compaction == nil
		if visible {
			l.emit(r, "turn_start", "", "")
			l.emit(r, "message_start", "", "")
		}
		callCtx, cancel := context.WithTimeout(l.execution, 45*time.Second)
		generate := l.options.Provider.Generate
		if cp.Step > 0 {
			generate = l.options.Provider.Continue
		}
		value, callErr := generate(callCtx, generation.Request, func(e llm.GenerationEvent) {
			if visible && e.Kind == "text_delta" {
				l.emit(r, "message_update", e.Text, "")
			}
		})
		cancel()
		if l.execution.Err() != nil {
			return false, l.execution.Err()
		}
		if callErr != nil {
			// Storage/claim rejection is infrastructure, not a failed model.
			if errors.Is(callErr, ErrClaimLost) || errors.Is(callErr, ErrStorage) || errors.Is(callErr, llm.ErrStorage) {
				return false, callErr
			}
			// A failed provider response may still report billable usage.
			// Settlement can have requested Root consent atomically with
			// that failure; preserve it before finalizing the Run.
			latest, loadErr := l.options.Store.Load(l.work, l.claim)
			if loadErr != nil {
				return false, loadErr
			}
			if latest.SpendStatus == "pending" {
				if err := l.options.Store.Wait(l.work, l.claim, latest.Version, "resource"); err != nil {
					return false, err
				}
				return true, nil
			}
			if latest.SpendStatus == "denied" || latest.SpendStatus == "invalidated" {
				return false, l.fail(latest, "SPEND_AUTHORIZATION_DENIED")
			}
			if errors.Is(callErr, llm.ContextLengthExceeded) {
				if retry, e := l.retryContext(latest); retry {
					if e != nil {
						return false, e
					}
					return false, nil
				}
			}
			return false, l.fail(latest, llm.NormalizeError(callErr.Error()).Error())
		}
		result = &value
	}
	if plan := generation.Compaction; plan != nil {
		if result.Clipped || len(result.ToolCalls) > 0 || strings.TrimSpace(result.Text) == "" || len(result.Text) > plan.MaxBytes {
			return false, l.fail(r, llm.InvalidProviderResponse.Error())
		}
		cp.WorkingState = &WorkingState{Through: plan.Through, Text: result.Text, SourceRefs: plan.SourceRefs, ArchiveRef: plan.ArchiveRef}
		cp.Step++
		cp.Result = nil
		cp.ExchangeID = ""
		cp.Phase = ModelPhase
		return false, l.options.Store.Checkpoint(l.work, l.claim, r.Version, cp)
	}
	cp.ContextRetries = 0
	// Bridge acceptance has durably saved the complete result already.
	parts := conversation.Text(result.Text)
	for _, call := range result.ToolCalls {
		c := conversation.ToolCall{ID: call.ID, Name: call.Name, Arguments: call.Arguments}
		parts = append(parts, conversation.ContentPart{Type: "tool_call", ToolCall: &c})
	}
	cp.Messages = append(cp.Messages, AgentMessage{Type: "assistant", Content: parts})
	cp.Result = result
	cp.ExchangeID = generation.Request.GenerationID
	cp.Phase = ReplyPhase
	if len(result.ToolCalls) > 0 {
		cp.Phase = ToolsPhase
	}
	if err = l.options.Store.Checkpoint(l.work, l.claim, r.Version, cp); err != nil {
		return false, err
	}
	l.emit(r, "message_end", "", "")
	return false, nil
}

func (l agentLoop) toolsTurn(r Run) (bool, error) {
	cp := r.Checkpoint
	results, err := l.options.Tools.Execute(l.execution, cp.ExchangeID, cp.Result.ToolCalls)
	if errors.Is(err, tool.ErrSuspended) {
		reason, waitErr := l.options.Tools.Store.WaitReason(l.work)
		if waitErr != nil {
			return false, waitErr
		}
		if err = l.options.Store.Wait(l.work, l.claim, r.Version, reason); err != nil {
			return false, err
		}
		return true, nil
	}
	if err != nil {
		return false, err
	}
	unknownExchange := false
	for _, result := range results {
		if result.Error != nil && result.Error.Code == "UNKNOWN_SIDE_EFFECT" {
			unknownExchange = true
		}
	}
	if unknownExchange {
		// Preserve the complete native exchange, including all sibling results,
		// rather than inserting a user message between Tool calls and replies.
		for i := len(cp.Messages) - 1; i >= 0; i-- {
			if cp.Messages[i].Type == "assistant" {
				cp.Messages[i].Source = &llm.ContextSource{Kind: "unknown_exchange", Protected: true}
				break
			}
		}
	}
	for i, result := range results {
		parts := conversation.Text(result.ModelText())
		for _, ref := range result.ImageRefs {
			parts = append(parts, conversation.ContentPart{Type: "image", ArtifactRef: ref})
		}
		message := AgentMessage{Type: "tool_result", ToolCallID: cp.Result.ToolCalls[i].ID, Content: parts}
		if unknownExchange {
			message.Source = &llm.ContextSource{Kind: "unknown_exchange", Protected: true}
		}
		cp.Messages = append(cp.Messages, message)
	}
	cp.Step++
	cp.Phase = ModelPhase
	cp.ExchangeID = ""
	cp.Result = nil
	if err = l.options.Store.Checkpoint(l.work, l.claim, r.Version, cp); err != nil {
		return false, err
	}
	l.emit(r, "turn_end", "", "")
	return false, nil
}

func (l agentLoop) replyTurn(r Run) (bool, error) {
	cp := r.Checkpoint
	ready, err := l.options.Store.PrepareReply(l.work, l.claim, r)
	if err != nil {
		return false, err
	}
	if !ready {
		// A completed generation may have been overtaken by user steering.
		// Keep it as model context, then inject separate user messages.
		cp.Step++
		cp.Phase = ModelPhase
		cp.Result = nil
		cp.ExchangeID = ""
		if err = l.options.Store.Checkpoint(l.work, l.claim, r.Version, cp); err != nil {
			return false, err
		}
		return false, nil
	}
	if err = l.options.Store.DeliverReply(l.work, l.claim, r); err != nil {
		return false, err
	}
	l.emit(r, "turn_end", "", "")
	return false, nil
}

func (l agentLoop) settleTurn(r Run) (bool, error) {
	cp := r.Checkpoint
	done, err := l.options.Store.Complete(l.work, l.claim, r.Version)
	if err != nil {
		return false, err
	}
	if done {
		l.emit(r, "agent_end", "", "")
		return true, nil
	}
	cp.Phase = ModelPhase
	if err = l.options.Store.Checkpoint(l.work, l.claim, r.Version, cp); err != nil {
		return false, err
	}
	return false, nil
}

func (l agentLoop) emit(r Run, kind, text, call string) {
	if l.options.Sink != nil {
		l.options.Sink(Event{Kind: kind, RunID: r.ID, AgentID: r.AgentID, Step: r.Checkpoint.Step, OccurredAt: time.Now().UTC(), Text: text, ToolCallID: call})
	}
}

func (l agentLoop) fail(r Run, code string) error {
	if err := l.options.Store.Fail(l.work, l.claim, r.Version, code); err != nil {
		return err
	}
	if code == "SPEND_AUTHORIZATION_DENIED" {
		return &RunFailure{cause: ErrSpendDenied}
	}
	if code == "RESOURCE_LIMIT" {
		return &RunFailure{cause: ErrResourceLimit}
	}
	return &RunFailure{cause: llm.NormalizeError(code)}
}

func (l agentLoop) retryContext(run Run) (bool, error) {
	if run.Checkpoint.ContextRetries >= 1 {
		return false, nil
	}
	cp := run.Checkpoint
	cp.ContextRetries++
	cp.Step++
	cp.Result = nil
	cp.ExchangeID = ""
	cp.Phase = ModelPhase
	return true, l.options.Store.Checkpoint(l.work, l.claim, run.Version, cp)
}
