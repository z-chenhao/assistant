package ui

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestUpstreamNativeCatalogAndBoundMeetingResponse(t *testing.T) {
	c := Component{Name: "scheduleTime", Props: json.RawMessage(`{"reasonForScheduling":"Review","meetingDuration":30,"timeSlots":[{"date":"2026-11-07","time":"10:00"},{"date":"2026-11-08","time":"14:00"}]}`)}
	s := nativeSpec(t, []Component{c, c})
	if s.Components[0].FormID == s.Components[1].FormID {
		t.Fatal("distinct pickers share identity")
	}
	c = s.Components[0]
	content, err := Answer(c, map[string]string{"choice": "0"})
	if err != nil || !strings.Contains(content[0].Text, "2026-11-07 at 10:00") || !strings.Contains(content[0].Text, "does not book") {
		t.Fatal(content, err)
	}
	if err = Restore(&c, content); err != nil || !c.Submitted || c.Response != "0" {
		t.Fatal(c, err)
	}
	for _, choice := range []string{"01", "2", "-1", "forged"} {
		if _, err := Answer(c, map[string]string{"choice": choice}); err == nil {
			t.Fatal("unknown choice accepted", choice)
		}
	}
	if _, err := Answer(c, map[string]string{"choice": "0", "approval": "yes"}); err == nil {
		t.Fatal("extra authority accepted")
	}
}

func TestRetiredAndMalformedPresentationInputsAreRejected(t *testing.T) {
	for _, raw := range []string{
		`{"title":"T","fallback":"F","html":"<p>legacy</p>"}`,
		`{"title":"T","fallback":"F","components":[{"type":"markdown","text":"old"}]}`,
		`{"title":"T","fallback":"F","components":[{"name":"Table","props":{"title":"T","columns":["A"],"rows":[["a","b"]],"source":"User"}}]}`,
		`{"title":"T","fallback":"F","components":[{"name":"pieChart","props":{"title":"T","description":"D","data":[{"label":"A","value":-1}]}}]}`,
		`{"title":"T","fallback":"F","components":[{"name":"scheduleTime","props":{"reasonForScheduling":"R","meetingDuration":0,"timeSlots":[{"date":"tomorrow","time":"10"}]}}]}`,
	} {
		if _, ok := ParseWidget(raw); ok {
			t.Fatal("invalid presentation accepted", raw)
		}
	}
	if _, ok := StoredWidget(`{"title":"T","fallback":"Historical text","html":"<p>old</p>"}`); !ok {
		t.Fatal("immutable history became unreadable")
	}
}
