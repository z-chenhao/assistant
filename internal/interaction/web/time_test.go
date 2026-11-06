package web

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/z-chenhao/assistant/internal/conversation"
)

func zonedRequest(target, zone string) *http.Request {
	r := httptest.NewRequest("GET", target, nil)
	r.AddCookie(&http.Cookie{Name: timezoneCookie, Value: zone})
	return r
}

func TestBrowserTimezoneRendersConversationAndLiveHistory(t *testing.T) {
	first, _ := time.Parse(time.RFC3339Nano, "2026-11-05T15:59:00Z")
	c := &updatesCore{allowed: true, status: "busy"}
	for i, at := range []time.Time{first, first.Add(time.Minute), first.Add(2 * time.Minute)} {
		c.messages = append(c.messages, conversation.Message{Role: "user", Sequence: int64(i + 1), CreatedAt: at, Content: conversation.Text("Keep the original message")})
	}
	s := &Server{Core: c}
	r := zonedRequest("/", "Asia/Shanghai")
	w := httptest.NewRecorder()
	s.show(w, r)
	body := w.Body.String()
	for _, want := range []string{">23:59:00</time>", ">00:00:00</time>", `datetime="2026-11-05T15:59:00Z"`, `data-live-key="date:3" hidden`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(body, " UTC") || strings.Contains(body, "GMT") || strings.Contains(body, "Asia/Shanghai") {
		t.Fatal("conversation displayed UTC")
	}
	p, err := s.page(r.Context(), r)
	if err != nil {
		t.Fatal(err)
	}
	var fragment bytes.Buffer
	if err := requestLayout(r).ExecuteTemplate(&fragment, "conversation", p); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fragment.String(), ">00:00:00</time>") || !c.messages[0].CreatedAt.Equal(first) {
		t.Fatal("live history changed the instant or lost the viewer zone")
	}
}

func TestTimeFiltersUseViewerZoneAndKeepExplicitInstants(t *testing.T) {
	for _, target := range []string{"/developer/observability?since=2026-11-06T08:30", "/developer/observability?since=2026-11-06T00:30:00Z"} {
		r := zonedRequest(target, "Asia/Shanghai")
		trace, err := traceQuery(r)
		if err != nil || trace.Query.Since.Format(time.RFC3339) != "2026-11-06T00:30:00Z" {
			t.Fatal("trace filter changed the selected instant", trace, err)
		}
		memory, err := memoryQuery(r)
		if err != nil || !memory.Since.Equal(trace.Query.Since) {
			t.Fatal("memory and trace filter disagree", memory, err)
		}
	}
	api, err := inspectionQuery(zonedRequest("/api/developer/executions?since=2026-11-06T08:30", "Asia/Shanghai"))
	if err != nil || api.Query.Since.Format(time.RFC3339) != "2026-11-06T08:30:00Z" {
		t.Fatal("legacy API filter semantics changed", api, err)
	}
	location, _ := time.LoadLocation("America/New_York")
	if _, err := inspectionTimeIn("2026-03-08T02:30", location); err == nil {
		t.Fatal("nonexistent daylight-saving time was silently shifted")
	}
	for _, raw := range []string{"2026-11-01T01:30:00-04:00", "2026-11-01T01:30:00-05:00"} {
		at, err := inspectionTimeIn(raw, location)
		if err != nil || at.IsZero() {
			t.Fatal(raw, err)
		}
		if !strings.Contains(string(localTimeHTML(raw, "datetime", location, "en")), raw[19:]) {
			t.Fatal("lost the original instant", raw)
		}
	}
}

func TestTimezoneIsRequestLocalAndInvalidPreferencesFallBack(t *testing.T) {
	for _, zone := range []string{"Asia/Shanghai", "America/New_York", "Asia/Kathmandu"} {
		r := zonedRequest("/", zone)
		var out bytes.Buffer
		if err := requestLayout(r).ExecuteTemplate(&out, "conversation", page{Messages: []shownMessage{{Role: "user", CreatedAt: "2026-11-06T00:00:00Z"}}}); err != nil {
			t.Fatal(err)
		}
		expected := string(localTimeHTML("2026-11-06T00:00:00Z", "message", requestLocation(r), "en"))
		if !strings.Contains(out.String(), expected) {
			t.Fatal("zone leaked between requests", zone, out.String())
		}
	}
	for _, zone := range []string{"invalid/zone", "%00", "../UTC"} {
		if requestLocation(zonedRequest("/", zone)) != time.Local {
			t.Fatal("invalid zone accepted", zone)
		}
	}
	if localTimeHTML("", "message", time.Local, "en") != "" || localTimeHTML("0001-01-01T00:00:00Z", "message", time.Local, "en") != "" {
		t.Fatal("fabricated a missing message time")
	}
}
