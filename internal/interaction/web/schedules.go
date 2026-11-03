package web

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/z-chenhao/assistant/internal/agent"
	"github.com/z-chenhao/assistant/internal/automation"
	"github.com/z-chenhao/assistant/internal/task"
)

type ScheduleSettings interface {
	Schedule(context.Context, string) (task.ScheduledWork, error)
	ConfigureSchedule(context.Context, string, automation.OwnerConfiguration) (automation.OwnerReceipt, error)
}
type scheduleEditor struct {
	Ref, Key, Instruction, Frequency, Date, Time, Timezone, Cron, Threshold, TaskRef string
	Version                                                                          int64
	Weekday, Day                                                                     string
	Enabled                                                                          bool
	OriginalSpec, OriginalTimezone                                                   string
}

func scheduleURL(ref string) string { return "/tasks/schedules/" + url.PathEscape(ref) }

func localScheduleTime(language, raw, zone string) string {
	at, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return raw
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return raw
	}
	format := "Jan 2, 2006 · 15:04"
	if language == "zh-CN" {
		format = "2006年1月2日 · 15:04"
	}
	return at.In(loc).Format(format) + " " + zone
}
func scheduleFrequency(kind, spec string) string {
	if kind == "at" {
		return "Scheduled once"
	}
	f := strings.Fields(spec)
	if len(f) != 5 {
		return "Custom cron"
	}
	m, me := strconv.Atoi(f[0])
	h, he := strconv.Atoi(f[1])
	if me != nil || he != nil || m < 0 || m > 59 || h < 0 || h > 23 {
		return "Custom cron"
	}
	switch {
	case f[2] == "*" && f[3] == "*" && f[4] == "*":
		return "Every day"
	case f[2] == "*" && f[3] == "*" && f[4] == "1-5":
		return "Weekdays"
	case f[2] == "*" && f[3] == "*" && len(f[4]) == 1 && f[4] >= "0" && f[4] <= "6":
		return "Weekly"
	case f[3] == "*" && f[4] == "*":
		if day, err := strconv.Atoi(f[2]); err == nil && day >= 1 && day <= 31 {
			return "Monthly"
		}
	}
	return "Custom cron"
}

func (s *Server) loadSchedule(ctx context.Context, r *http.Request, p *page) error {
	if s.Schedules == nil {
		return errPageNotFound
	}
	e := &scheduleEditor{Key: sourceID(), Frequency: "daily", Time: "08:00", Timezone: "UTC", Enabled: true, Weekday: "1", Day: "1"}
	p.Schedule = e
	if r.URL.Path == "/tasks/schedules/new" {
		return nil
	}
	ref := strings.TrimPrefix(r.URL.Path, "/tasks/schedules/")
	v, err := s.Schedules.Schedule(ctx, ref)
	if err != nil {
		return err
	}
	if v.Restricted {
		return task.ErrNotFound
	}
	e.Ref, e.Version, e.Instruction, e.Timezone, e.Enabled, e.TaskRef = v.Ref, v.Version, v.Instruction, v.Timezone, v.Enabled, v.TaskRef
	if v.Below != nil {
		e.Threshold = strconv.FormatInt(*v.Below, 10)
	}
	e.Frequency = "custom"
	e.Cron = v.Spec
	if v.Type == "at" {
		e.OriginalSpec, e.OriginalTimezone = v.Spec, v.Timezone
		e.Frequency = "once"
		at, err := time.Parse(time.RFC3339Nano, v.Spec)
		if err != nil {
			return err
		}
		loc, err := time.LoadLocation(v.Timezone)
		if err != nil {
			return err
		}
		e.Date, e.Time = at.In(loc).Format("2006-01-02"), at.In(loc).Format("15:04")
	} else {
		fields := strings.Fields(v.Spec)
		if len(fields) == 5 {
			minute, em := strconv.Atoi(fields[0])
			hour, eh := strconv.Atoi(fields[1])
			if em == nil && eh == nil && minute >= 0 && minute < 60 && hour >= 0 && hour < 24 {
				e.Time = time.Date(2000, 1, 1, hour, minute, 0, 0, time.UTC).Format("15:04")
				switch {
				case fields[2] == "*" && fields[3] == "*" && fields[4] == "*":
					e.Frequency = "daily"
				case fields[2] == "*" && fields[3] == "*" && fields[4] == "1-5":
					e.Frequency = "weekdays"
				case fields[2] == "*" && fields[3] == "*" && len(fields[4]) == 1 && fields[4] >= "0" && fields[4] <= "6":
					e.Frequency = "weekly"
					e.Weekday = fields[4]
				case fields[3] == "*" && fields[4] == "*":
					if day, err := strconv.Atoi(fields[2]); err == nil && day >= 1 && day <= 31 {
						e.Frequency = "monthly"
						e.Day = fields[2]
					}
				}
			}
		}
	}
	return nil
}

// A local date/time must identify exactly one instant. Go's ParseInLocation
// silently normalizes DST gaps/folds, so collect matching zone offsets instead.
func exactLocalOnce(date, clock, zone string) (string, error) {
	loc, err := time.LoadLocation(zone)
	if err != nil || zone == "" || zone == "Local" {
		return "", automation.ErrInvalid
	}
	wall, err := time.Parse("2006-01-02T15:04", date+"T"+clock)
	if err != nil {
		return "", automation.ErrInvalid
	}
	offsets := map[int]bool{}
	for h := -36; h <= 36; h++ {
		_, offset := wall.Add(time.Duration(h) * time.Hour).In(loc).Zone()
		offsets[offset] = true
	}
	var matches []time.Time
	for offset := range offsets {
		candidate := wall.Add(-time.Duration(offset) * time.Second)
		if candidate.In(loc).Format("2006-01-02T15:04") == date+"T"+clock {
			matches = append(matches, candidate)
		}
	}
	if len(matches) != 1 {
		return "", automation.ErrInvalid
	}
	return matches[0].UTC().Format(time.RFC3339), nil
}
func decodeScheduleForm(r *http.Request) (*scheduleEditor, automation.OwnerConfiguration, error) {
	e := &scheduleEditor{}
	q := automation.OwnerConfiguration{}
	allowed := map[string]bool{"ref": true, "version": true, "source_id": true, "instruction": true, "frequency": true, "date": true, "time": true, "timezone": true, "cron": true, "threshold": true, "weekday": true, "day": true, "enabled": true, "original_spec": true, "original_timezone": true}
	get := r.PostForm.Get
	e.Ref, e.Key, e.Instruction, e.Frequency, e.Date, e.Time, e.Timezone, e.Cron, e.Threshold, e.Weekday, e.Day = get("ref"), get("source_id"), get("instruction"), get("frequency"), get("date"), get("time"), get("timezone"), get("cron"), get("threshold"), get("weekday"), get("day")
	e.Enabled = get("enabled") == "yes"
	e.OriginalSpec, e.OriginalTimezone = get("original_spec"), get("original_timezone")
	var err error
	e.Version, err = strconv.ParseInt(get("version"), 10, 64)
	// Retain the submitted editor before enforcing byte limits: maxlength counts
	// characters, and ordinary CJK input may exceed the domain's UTF-8 byte limit.
	for key, values := range r.PostForm {
		if !allowed[key] || len(values) != 1 || len(values[0]) > 8192 {
			return e, q, automation.ErrInvalid
		}
	}
	if err != nil || len(e.Key) != 32 || strings.Trim(e.Key, "0123456789abcdef") != "" || (get("enabled") != "" && get("enabled") != "yes") {
		return e, q, automation.ErrInvalid
	}
	q = automation.OwnerConfiguration{Ref: e.Ref, Version: e.Version, Instruction: e.Instruction, Timezone: e.Timezone, Enabled: e.Enabled}
	if e.Threshold != "" {
		value, err := strconv.ParseInt(e.Threshold, 10, 64)
		if err != nil {
			return e, q, automation.ErrInvalid
		}
		q.Below = &value
	}
	switch e.Frequency {
	case "once":
		q.Type = "at"
		// A new owner edit proves this original against stored Spec and zone inside
		// the transaction. Retrying carries the same exact command to its receipt.
		if e.Ref != "" && e.OriginalSpec != "" && e.OriginalTimezone == e.Timezone {
			at, parseErr := time.Parse(time.RFC3339Nano, e.OriginalSpec)
			loc, zoneErr := time.LoadLocation(e.Timezone)
			if parseErr == nil && zoneErr == nil && at.In(loc).Format("2006-01-02") == e.Date && at.In(loc).Format("15:04") == e.Time {
				q.Spec, q.PreserveSpec = e.OriginalSpec, e.OriginalSpec
			}
		}
		if q.Spec == "" {
			q.Spec, err = exactLocalOnce(e.Date, e.Time, e.Timezone)
		}
	case "custom":
		q.Type = "cron"
		q.Spec = e.Cron
	case "daily", "weekdays", "weekly", "monthly":
		clock, parseErr := time.Parse("15:04", e.Time)
		if parseErr != nil {
			err = automation.ErrInvalid
			break
		}
		day, weekday := "*", "*"
		if e.Frequency == "weekdays" {
			weekday = "1-5"
		}
		if e.Frequency == "weekly" {
			value, parseErr := strconv.Atoi(e.Weekday)
			if parseErr != nil || value < 0 || value > 6 {
				err = automation.ErrInvalid
				break
			}
			weekday = strconv.Itoa(value)
		}
		if e.Frequency == "monthly" {
			value, parseErr := strconv.Atoi(e.Day)
			if parseErr != nil || value < 1 || value > 31 {
				err = automation.ErrInvalid
				break
			}
			day = strconv.Itoa(value)
		}
		q.Type = "cron"
		q.Spec = strconv.Itoa(clock.Minute()) + " " + strconv.Itoa(clock.Hour()) + " " + day + " * " + weekday
	default:
		err = automation.ErrInvalid
	}
	if err == nil {
		err = q.Validate()
	}
	return e, q, err
}
func (s *Server) configureSchedule(w http.ResponseWriter, r *http.Request) error {
	if s.Schedules == nil {
		return errPageNotFound
	}
	e, q, err := decodeScheduleForm(r)
	if err == nil {
		var receipt automation.OwnerReceipt
		receipt, err = s.Schedules.ConfigureSchedule(r.Context(), e.Key, q)
		if err == nil {
			http.Redirect(w, r, scheduleURL(receipt.Ref)+"?notice=schedule-saved", http.StatusSeeOther)
			return nil
		}
	}
	message := "Check the instructions, schedule, time zone and threshold. One-time local times must be valid and unambiguous; enabled occurrences must be in the future."
	if errors.Is(err, agent.ErrConflict) {
		message = "This schedule changed or can no longer be edited. Your draft is preserved. Reload the saved schedule before making a new change."
	}
	if err != nil && !errors.Is(err, automation.ErrInvalid) && !errors.Is(err, agent.ErrConflict) {
		message = "The schedule could not be saved. Your draft is preserved; try again with the same form."
	}
	p := page{Tab: "/tasks", View: "schedule", Title: "Schedule settings", Path: r.URL.Path, Schedule: e, Error: message}
	if status, statusErr := s.Core.WebStatus(r.Context()); statusErr == nil {
		p.Status = status
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusConflict)
	s.render(w, r, p)
	return nil
}
