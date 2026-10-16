package agent

import (
	"context"
	"errors"
	"time"

	"github.com/z-chenhao/erii/internal/conversation"
	"github.com/z-chenhao/erii/internal/llm"
	"github.com/z-chenhao/erii/internal/tool"
)

type LoopOptions struct {
	Store    RunStore
	Provider llm.LLMProvider
	Tools    tool.Executor
	Sink     EventSink
}

// RunAgentLoop and ContinueAgentLoop use exactly the same stateless kernel.
// Roles supply context, tools and a result destination outside this algorithm.
func RunAgentLoop(ctx context.Context, claim RunClaim, options LoopOptions) error {
	return ContinueAgentLoop(ctx, claim, options)
}

func ContinueAgentLoop(ctx context.Context, claim RunClaim, o LoopOptions) error {
	work := context.WithoutCancel(ctx)
	emit := func(r Run, kind, text, call string) {
		if o.Sink != nil {
			o.Sink(Event{Kind: kind, RunID: r.ID, AgentID: r.AgentID, Step: r.Checkpoint.Step, OccurredAt: time.Now().UTC(), Text: text, ToolCallID: call})
		}
	}
	r, err := o.Store.Load(work, claim)
	if err != nil {
		return err
	}
	emit(r, "agent_start", "", "")
	fail := func(r Run, code string) error {
		if err := o.Store.Fail(work, claim, r.Version, code); err != nil {
			return err
		}
		return llm.NormalizeError(code)
	}
	for {
		// Shutdown is a boundary stop. It never mutates business cancellation.
		if ctx.Err() != nil {
			return ctx.Err()
		}
		r, err = o.Store.Load(work, claim)
		if err != nil {
			return err
		}
		cp := r.Checkpoint
		switch cp.Phase {
		case "model":
			inputs, err := o.Store.Steering(work, claim, cp.InputSequence)
			if err != nil {
				return err
			}
			if len(inputs) > 0 {
				for _, input := range inputs {
					cp.Messages = append(cp.Messages, AgentMessage{Type: "user", Content: input.Message.Content})
					cp.InputSequence = input.Sequence
				}
				if err = o.Store.Checkpoint(work, claim, r.Version, cp); err != nil {
					return err
				}
				continue
			}
			generation, err := o.Store.Generation(work, claim, r)
			if err != nil {
				var providerError llm.ProviderError
				if errors.As(err, &providerError) {
					return fail(r, providerError.Error())
				}
				return err
			}
			if generation.ErrorCode != "" {
				return fail(r, generation.ErrorCode)
			}
			result := generation.Result
			if result == nil {
				emit(r, "turn_start", "", "")
				emit(r, "message_start", "", "")
				callCtx, cancel := context.WithTimeout(work, 45*time.Second)
				generate := o.Provider.Generate
				if cp.Step > 0 {
					generate = o.Provider.Continue
				}
				value, callErr := generate(callCtx, generation.Request, func(e llm.GenerationEvent) {
					if e.Kind == "text_delta" {
						emit(r, "message_update", e.Text, "")
					}
				})
				cancel()
				if callErr != nil {
					// Storage/claim rejection is infrastructure, not a failed model.
					if callErr == ErrClaimLost || callErr == ErrStorage || callErr == llm.ErrStorage {
						return callErr
					}
					return fail(r, llm.NormalizeError(callErr.Error()).Error())
				}
				result = &value
			}
			// Bridge acceptance has durably saved the complete result already.
			parts := conversation.Text(result.Text)
			for _, call := range result.ToolCalls {
				c := conversation.ToolCall{ID: call.ID, Name: call.Name, Arguments: call.Arguments}
				parts = append(parts, conversation.ContentPart{Type: "tool_call", ToolCall: &c})
			}
			cp.Messages = append(cp.Messages, AgentMessage{Type: "assistant", Content: parts})
			cp.Result = result
			cp.ExchangeID = generation.Request.GenerationID
			cp.Phase = "reply"
			if len(result.ToolCalls) > 0 {
				cp.Phase = "tools"
			}
			if err = o.Store.Checkpoint(work, claim, r.Version, cp); err != nil {
				return err
			}
			emit(r, "message_end", "", "")
		case "tools":
			results, err := o.Tools.Execute(work, cp.ExchangeID, cp.Result.ToolCalls)
			if err != nil {
				return err
			}
			for i, result := range results {
				cp.Messages = append(cp.Messages, AgentMessage{Type: "tool_result", ToolCallID: cp.Result.ToolCalls[i].ID, Content: conversation.Text(result.ModelText())})
			}
			cp.Step++
			cp.Phase = "model"
			cp.ExchangeID = ""
			cp.Result = nil
			if err = o.Store.Checkpoint(work, claim, r.Version, cp); err != nil {
				return err
			}
			emit(r, "turn_end", "", "")
		case "reply":
			ready, err := o.Store.PrepareReply(work, claim, r)
			if err != nil {
				return err
			}
			if !ready {
				// A completed generation may have been overtaken by user steering.
				// Keep it as model context, then inject separate user messages.
				cp.Step++
				cp.Phase = "model"
				cp.Result = nil
				cp.ExchangeID = ""
				if err = o.Store.Checkpoint(work, claim, r.Version, cp); err != nil {
					return err
				}
				continue
			}
			if err = o.Store.DeliverReply(work, claim, r); err != nil {
				return err
			}
			emit(r, "turn_end", "", "")
		case "settle":
			done, err := o.Store.Complete(work, claim, r.Version)
			if err != nil {
				return err
			}
			if done {
				emit(r, "agent_end", "", "")
				return nil
			}
			cp.Phase = "model"
			if err = o.Store.Checkpoint(work, claim, r.Version, cp); err != nil {
				return err
			}
		default:
			return ErrStorage
		}
	}
}
