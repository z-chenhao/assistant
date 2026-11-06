package agent

import (
	"context"
	"testing"
	"time"

	"github.com/z-chenhao/assistant/internal/delivery/ui"
)

func TestUIPreviewCancellationWithdrawsBeforeToolsJoin(t *testing.T) {
	execution, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan Event, 8)
	l := agentLoop{execution: execution, options: LoopOptions{Sink: func(e Event) { events <- e }}}
	r := Run{ID: "private-run", Kind: MainRole}
	l.beginUIPreview(r)
	if e := <-events; e.Kind != "ui_preview_start" {
		t.Fatal(e)
	}
	ui.ObservePreview(l.execution, "call", `{"title":"A","components":[]}`)
	if e := <-events; e.Kind != "ui_preview_update" {
		t.Fatal(e)
	}
	cancel()
	select {
	case e := <-events:
		if e.Kind != "ui_preview_discarded" {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("preview waited for tools/provider cleanup")
	}
	ui.ObservePreview(l.execution, "late", `{"title":"Late"}`)
	if len(events) != 0 {
		t.Fatal("cancelled observer published a late update")
	}
	l.endUIPreview(r, "ui_preview_discarded", "")
	if l.execution != execution || l.uiBase != nil || l.uiStop != nil {
		t.Fatal("preview retained its observer or cancellation watcher")
	}
}
func TestUIPreviewCommitReleasesObserverBetweenTurns(t *testing.T) {
	execution := context.Background()
	l := agentLoop{execution: execution}
	r := Run{ID: "run", Kind: MainRole}
	for i := 0; i < 3; i++ {
		l.beginUIPreview(r)
		l.endUIPreview(r, "ui_preview_committed", "generation")
		if l.execution != execution {
			t.Fatal("turns accumulated preview contexts")
		}
	}
}
