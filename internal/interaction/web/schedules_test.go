package web

import (
	"context"
	"errors"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/z-chenhao/assistant/internal/agent"
	"github.com/z-chenhao/assistant/internal/automation"
	"github.com/z-chenhao/assistant/internal/task"
)

func scheduleValues() url.Values {
	return url.Values{"ref": {""}, "version": {"0"}, "source_id": {strings.Repeat("a", 32)}, "instruction": {"Review the actual report"}, "frequency": {"daily"}, "date": {"2030-11-04"}, "time": {"08:30"}, "timezone": {"Asia/Shanghai"}, "cron": {"*/15 9-17 * * 1-5"}, "threshold": {"0"}, "weekday": {"0"}, "day": {"31"}, "enabled": {"yes"}}
}
func decodedSchedule(values url.Values, saved ...task.ScheduledWork) (*scheduleEditor, automation.OwnerConfiguration, error) {
	r := httptest.NewRequest("POST", "http://127.0.0.1:8080/schedule/configure", strings.NewReader(values.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.ParseForm()
	if len(saved) == 1 {
		r.PostForm.Set("original_spec", saved[0].Spec)
		r.PostForm.Set("original_timezone", saved[0].Timezone)
	}
	return decodeScheduleForm(r)
}

func TestScheduleCodecEncodesFriendlyFrequencyAndRejectsAmbiguousLocalOnce(t *testing.T) {
	for _, test := range []struct{ frequency, spec, kind string }{{"once", "2030-11-04T00:30:00Z", "at"}, {"daily", "30 8 * * *", "cron"}, {"weekdays", "30 8 * * 1-5", "cron"}, {"weekly", "30 8 * * 0", "cron"}, {"monthly", "30 8 31 * *", "cron"}, {"custom", "*/15 9-17 * * 1-5", "cron"}} {
		v := scheduleValues()
		v.Set("frequency", test.frequency)
		_, q, err := decodedSchedule(v)
		if err != nil || q.Spec != test.spec || q.Type != test.kind || q.Below == nil || *q.Below != 0 {
			t.Fatal(test, q, err)
		}
	}
	for _, date := range []string{"2026-03-08", "2026-11-01"} {
		v := scheduleValues()
		v.Set("frequency", "once")
		v.Set("timezone", "America/New_York")
		v.Set("date", date)
		if strings.Contains(date, "03-") {
			v.Set("time", "02:30")
		} else {
			v.Set("time", "01:30")
		}
		if _, _, err := decodedSchedule(v); err == nil {
			t.Fatal("gap or fold accepted", date)
		}
	}
	for _, field := range []string{"version", "source_id", "frequency", "instruction"} {
		v := scheduleValues()
		v.Add(field, "duplicate")
		if _, _, err := decodedSchedule(v); err == nil {
			t.Fatal("duplicate field accepted", field)
		}
	}
	v := scheduleValues()
	v.Set("threshold", "")
	_, q, err := decodedSchedule(v)
	if err != nil || q.Below != nil {
		t.Fatal("blank threshold became zero", q, err)
	}
}

func TestScheduleCodecPreservesVersionBoundExistingOncePrecisionAndFold(t *testing.T) {
	for _, test := range []struct{ spec, zone, date, clock string }{{"2030-11-04T08:30:45.123456789+08:00", "Asia/Shanghai", "2030-11-04", "08:30"}, {"2026-11-01T01:30:45.123456789-04:00", "America/New_York", "2026-11-01", "01:30"}} {
		v := scheduleValues()
		v.Set("ref", "automation:existing")
		v.Set("version", "7")
		v.Set("frequency", "once")
		v.Set("date", test.date)
		v.Set("time", test.clock)
		v.Set("timezone", test.zone)
		v.Del("enabled")
		saved := task.ScheduledWork{Schedule: task.Schedule{Ref: "automation:existing", Version: 7, Type: "at", Spec: test.spec, Timezone: test.zone}}
		_, q, err := decodedSchedule(v, saved)
		if err != nil || q.Spec != test.spec || q.Enabled {
			t.Fatal("unchanged once instant lost precision/offset", q, err)
		}
	}
	if got := localScheduleTime("en", "2030-11-04T00:30:00Z", "Asia/Shanghai"); !strings.Contains(got, "08:30 Asia/Shanghai") {
		t.Fatal("saved zone not used", got)
	}
}

func TestScheduleCodecRetainsMultibyteDraftOnByteLimit(t *testing.T) {
	v := scheduleValues()
	v.Set("ref", "automation:existing")
	v.Set("version", "7")
	v.Set("instruction", strings.Repeat("研究进展", 800))
	e, _, err := decodedSchedule(v)
	if !errors.Is(err, automation.ErrInvalid) || e.Instruction != v.Get("instruction") || e.Key != v.Get("source_id") || e.Ref != "automation:existing" || e.Version != 7 {
		t.Fatal("multibyte validation erased the retryable draft", e, err)
	}
}

type scheduleServiceFixture struct {
	q          automation.OwnerConfiguration
	key        string
	calls      int
	err        error
	restricted bool
}

func (f *scheduleServiceFixture) Schedule(_ context.Context, ref string) (task.ScheduledWork, error) {
	return task.ScheduledWork{Schedule: task.Schedule{Ref: ref, Version: 7, Instruction: "Saved instruction", Type: "at", Spec: "2030-11-04T08:30:45.123456789+08:00", Timezone: "Asia/Shanghai", Enabled: true, Restricted: f.restricted}}, nil
}
func (f *scheduleServiceFixture) ConfigureSchedule(_ context.Context, key string, q automation.OwnerConfiguration) (automation.OwnerReceipt, error) {
	f.calls++
	f.q = q
	f.key = key
	return automation.OwnerReceipt{Ref: "automation:saved", Version: 8}, f.err
}

func TestScheduleFormsUseOwnerWriterAndPreserveConflictingDraft(t *testing.T) {
	f := &scheduleServiceFixture{}
	s := &Server{Core: &updatesCore{allowed: true, status: "idle"}, Tasks: workspaceReader{}, Schedules: f}
	request := func(method, path string, v url.Values, cross bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://127.0.0.1:8080"+path, strings.NewReader(v.Encode()))
		r.RemoteAddr = "127.0.0.1:9000"
		if method == "POST" {
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		if cross {
			r.Header.Set("Origin", "http://other.example")
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	if w := request("GET", "/tasks/schedules/new", nil, false); w.Code != 200 || !strings.Contains(w.Body.String(), "Save schedule") {
		t.Fatal(w.Code, w.Body.String())
	}
	v := scheduleValues()
	if w := request("POST", "/schedule/configure", v, true); w.Code != 403 || f.calls != 0 {
		t.Fatal("cross-origin writer reached", w.Code, f.calls)
	}
	if w := request("POST", "/schedule/configure", v, false); w.Code != 303 || !strings.Contains(w.Header().Get("Location"), "automation:saved") || f.calls != 1 || f.q.Spec != "30 8 * * *" {
		t.Fatal(w.Code, w.Header(), f.q)
	}
	f.err = agent.ErrConflict
	v.Set("ref", "automation:existing")
	v.Set("version", "7")
	v.Set("instruction", "Draft <img src=x>")
	v.Set("frequency", "once")
	v.Set("original_spec", "2030-11-04T08:30:45.123456789+08:00")
	v.Set("original_timezone", "Asia/Shanghai")
	w := request("POST", "/schedule/configure", v, false)
	if w.Code != 409 || !strings.Contains(w.Body.String(), "Draft &lt;img src=x&gt;") || !strings.Contains(w.Body.String(), `name="version" value="7"`) || !strings.Contains(w.Body.String(), strings.Repeat("a", 32)) || f.q.Spec != "2030-11-04T08:30:45.123456789+08:00" {
		t.Fatal("conflict lost draft, version, key or exact instant", w.Code, w.Body.String(), f.q)
	}
	f.err = errors.New("PRIVATE_OWNER_ERROR")
	w = request("POST", "/schedule/configure", v, false)
	if w.Code != 409 || strings.Contains(w.Body.String(), "PRIVATE_OWNER_ERROR") || !strings.Contains(w.Body.String(), "Draft &lt;img src=x&gt;") {
		t.Fatal("private failure leaked or lost draft", w.Code)
	}
	f.restricted = true
	if w = request("GET", "/tasks/schedules/automation:existing", nil, false); w.Code != 404 {
		t.Fatal("revoked context editable", w.Code)
	}
}
