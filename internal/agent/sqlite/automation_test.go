package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/z-chenhao/erii/internal/agent"
	"github.com/z-chenhao/erii/internal/automation"
	"github.com/z-chenhao/erii/internal/llm"
	"github.com/z-chenhao/erii/internal/memory"
	memorysqlite "github.com/z-chenhao/erii/internal/memory/sqlite"
	"github.com/z-chenhao/erii/internal/tool"
)

// Historical upgrade fixtures first remove the later, empty Automation schema.
const removeAutomationFixtureSchema = `DROP TABLE automation_request; DROP TABLE automation_fire; DROP TABLE automation; `

func automationFixture(t *testing.T) (*Store, *Runs, agent.Actor, *loopProvider) {
	s, r, _, a, p := loopFixture(t)
	echo, _ := tool.EchoRegistry().Resolve("builtin__text__echo")
	defs := append([]tool.Definition{echo}, automation.Definitions()...)
	registry, e := tool.NewRegistry(defs)
	if e != nil {
		t.Fatal(e)
	}
	r.registry = registry
	a.Registry = registry
	return s, r, a, p
}
func createAutomation(t *testing.T, r *Runs, now time.Time, below *int64) string {
	t.Helper()
	result, e := r.ManageAutomation(context.Background(), "create", automation.Request{Operation: "create", Instruction: "HKG-CDG 2026-12-01 price", Type: "cron", Spec: "0 8 * * *", Timezone: "Asia/Shanghai", Below: below}, now)
	if e != nil || result.Error != nil {
		t.Fatalf("create %v %+v", e, result)
	}
	var v struct {
		Ref string `json:"automationRef"`
	}
	if json.Unmarshal([]byte(result.Text), &v) != nil || v.Ref == "" {
		t.Fatal(result)
	}
	return v.Ref
}
func TestAutomationThresholdCrossingsAndCancel(t *testing.T) {
	ctx := context.Background()
	s, r, a, p := automationFixture(t)
	now, _ := time.Parse(time.RFC3339, "2026-10-15T12:00:00+08:00")
	threshold := int64(1800)
	id := createAutomation(t, r, now, &threshold)
	price := int64(0)
	p.respond = func(req llm.GenerationRequest) (llm.GenerationResult, error) {
		worker := false
		for _, m := range req.Messages {
			worker = worker || strings.Contains(m.Text, "general Worker executing")
		}
		last := req.Messages[len(req.Messages)-1]
		if worker && last.Role != "tool" {
			return llm.GenerationResult{ToolCalls: []llm.ToolCall{{ID: "price", Name: automation.ObserveName, Arguments: fmtValue(price)}, {ID: "second-source", Name: automation.ObserveName, Arguments: fmtValue(price)}}}, nil
		}
		return llm.GenerationResult{Text: "observed price"}, nil
	}
	for i, value := range []int64{2200, 2190, 1750, 1740, 2050, 1780} {
		price = value
		day := now.AddDate(0, 0, i+1)
		r.SetTestAutomationClock(day)
		if e := r.ScheduleAutomations(ctx, day); e != nil {
			t.Fatal(e)
		}
		if e := r.ScheduleAutomations(ctx, day); e != nil {
			t.Fatal(e)
		}
		mustDrain(t, a)
		if got := scalar(t, s, `SELECT count(*) FROM run WHERE kind='worker'`); got != i+1 {
			t.Fatalf("day %d roots %d", i, got)
		}
	}
	if scalar(t, s, `SELECT count(*) FROM task_update`) != 2 || scalar(t, s, `SELECT count(*) FROM automation_fire WHERE notify=1`) != 2 {
		t.Fatal("notifications must only be Oct18/Oct21")
	}
	if scalar(t, s, `SELECT count(*) FROM task WHERE status='active' AND completion_policy='ongoing'`) != 1 {
		t.Fatal("ongoing Task lost")
	}
	var version int64
	s.db.QueryRow(`SELECT version FROM automation WHERE automation_id=?`, id).Scan(&version)
	// Editing the schedule preserves an already-inside threshold state. A new
	// daily check below the same threshold must remain quiet.
	edit := automation.Request{Operation: "update", Ref: id, Version: version, Instruction: "HKG-CDG 2026-12-01 price", Type: "cron", Spec: "0 9 * * *", Timezone: "Asia/Shanghai", Below: &threshold}
	if result, e := r.ManageAutomation(ctx, "edit-time", edit, now.AddDate(0, 0, 6)); e != nil || result.Error != nil {
		t.Fatal(result, e)
	}
	price = 1700
	r.SetTestAutomationClock(now.AddDate(0, 0, 7))
	mustDrain(t, a)
	if scalar(t, s, `SELECT count(*) FROM task_update`) != 2 {
		t.Fatal("schedule edit repeated threshold notification")
	}
	s.db.QueryRow(`SELECT version FROM automation WHERE automation_id=?`, id).Scan(&version)
	if result, e := r.ManageAutomation(ctx, "cancel", automation.Request{Operation: "cancel", Ref: id, Version: version}, now.AddDate(0, 0, 7)); e != nil || result.Error != nil {
		t.Fatal(e, result)
	}
	r.SetTestAutomationClock(now.AddDate(0, 0, 20))
	mustDrain(t, a)
	if scalar(t, s, `SELECT count(*) FROM run WHERE kind='worker'`) != 7 {
		t.Fatal("cancel created a Root")
	}
}
func fmtValue(v int64) string {
	raw, _ := json.Marshal(struct {
		Value int64 `json:"value"`
	}{v})
	return string(raw)
}
func TestAutomationAtomicFireDelayedCancelAndCAS(t *testing.T) {
	ctx := context.Background()
	s, r, _, _ := automationFixture(t)
	now := time.Now().UTC()
	id := createAutomation(t, r, now, nil)
	fire := now.AddDate(0, 0, 3)
	// Transaction failure after Outbox write must leave schedule and fires intact.
	_, e := s.db.Exec(`CREATE TRIGGER reject_fire BEFORE INSERT ON event_outbox WHEN NEW.type='ai.erii.automation_fired.v1' BEGIN SELECT RAISE(ABORT,'fault'); END;`)
	if e != nil {
		t.Fatal(e)
	}
	if e = r.ScheduleAutomations(ctx, fire); !errors.Is(e, agent.ErrStorage) {
		t.Fatalf("fault %v", e)
	}
	if scalar(t, s, `SELECT count(*) FROM automation_fire`) != 0 {
		t.Fatal("partial fire")
	}
	s.db.Exec(`DROP TRIGGER reject_fire`)
	if e = r.ScheduleAutomations(ctx, fire); e != nil {
		t.Fatal(e)
	}
	if scalar(t, s, `SELECT count(*) FROM run`) != 0 {
		t.Fatal("Scheduler executed work")
	}
	// Missed days coalesce; schedule advances past the scan time.
	if scalar(t, s, `SELECT count(*) FROM automation_fire`) != 1 {
		t.Fatal("missed fire storm")
	}
	if result, e := r.ManageAutomation(ctx, "stale", automation.Request{Operation: "cancel", Ref: id, Version: 1}, fire); e != nil || result.Error == nil {
		t.Fatal("stale CAS accepted", result, e)
	}
	var v int64
	s.db.QueryRow(`SELECT version FROM automation WHERE automation_id=?`, id).Scan(&v)
	if _, e = r.ManageAutomation(ctx, "cancel", automation.Request{Operation: "cancel", Ref: id, Version: v}, fire); e != nil {
		t.Fatal(e)
	}
	if e = r.admitAutomations(ctx); e != nil {
		t.Fatal(e)
	}
	if scalar(t, s, `SELECT count(*) FROM run`) != 0 || scalar(t, s, `SELECT count(*) FROM automation_fire WHERE state='skipped'`) != 1 {
		t.Fatal("delayed cancel admitted work")
	}
}
func TestAutomationConcurrentSchedulersDeduplicate(t *testing.T) {
	ctx := context.Background()
	s, r, _, _ := automationFixture(t)
	now := time.Now()
	createAutomation(t, r, now, nil)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- r.ScheduleAutomations(ctx, now.AddDate(0, 0, 1)) }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	if scalar(t, s, `SELECT count(*) FROM automation_fire`) != 1 {
		t.Fatal("duplicate fire")
	}
}

func TestAutomationScheduleEditInvalidatesDelayedFireAndFreezesAtAdmission(t *testing.T) {
	ctx := context.Background()
	s, r, _, _ := automationFixture(t)
	now := time.Now()
	id := createAutomation(t, r, now, nil)
	fire := now.AddDate(0, 0, 1)
	if e := r.ScheduleAutomations(ctx, fire); e != nil {
		t.Fatal(e)
	}
	var v int64
	if e := s.db.QueryRow(`SELECT version FROM automation WHERE automation_id=?`, id).Scan(&v); e != nil {
		t.Fatal(e)
	}
	q := automation.Request{Operation: "update", Ref: id, Version: v, Instruction: "Edited reminder", Type: "at", Spec: fire.Add(time.Hour).Format(time.RFC3339), Timezone: "UTC"}
	result, e := r.ManageAutomation(ctx, "edit", q, fire)
	if e != nil || result.Error != nil {
		t.Fatal(e, result)
	}
	if e = r.admitAutomations(ctx); e != nil {
		t.Fatal(e)
	}
	if scalar(t, s, `SELECT count(*) FROM run`) != 0 {
		t.Fatal("old schedule admitted")
	}
	if e = r.ScheduleAutomations(ctx, fire.Add(2*time.Hour)); e != nil {
		t.Fatal(e)
	}
	// Profile changes after trigger but before admission affect only this new Run.
	input := modelInput()
	input.Name = "new admission model"
	input.ModelID = "new-model"
	profile := activateLoop(t, s, input)
	if e = r.admitAutomations(ctx); e != nil {
		t.Fatal(e)
	}
	if scalar(t, s, `SELECT count(*) FROM run r JOIN model_snapshot m ON m.snapshot_id=r.model_snapshot_id WHERE m.profile_id=?`, profile.ProfileID) != 1 {
		t.Fatal("snapshot was not frozen at admission")
	}
	if scalar(t, s, `SELECT count(*) FROM automation_fire WHERE state='skipped'`) != 1 {
		t.Fatal("old schedule not fenced")
	}
	if e = r.admitAutomations(ctx); e != nil {
		t.Fatal(e)
	}
	if scalar(t, s, `SELECT count(*) FROM run`) != 1 {
		t.Fatal("duplicate admission")
	}
	var root string
	if e = s.db.QueryRow(`SELECT run_id FROM run`).Scan(&root); e != nil {
		t.Fatal(e)
	}
	claim, e := r.Claim(ctx, root)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.db.QueryRow(`SELECT version FROM automation WHERE automation_id=?`, id).Scan(&v); e != nil {
		t.Fatal(e)
	}
	if _, e = r.ManageAutomation(ctx, "cancel", automation.Request{Operation: "cancel", Ref: id, Version: v}, fire); e != nil {
		t.Fatal(e)
	}
	if _, e = r.Load(ctx, claim); !errors.Is(e, agent.ErrClaimLost) {
		t.Fatalf("cancel did not fence old owner %v", e)
	}
}

// An edit can commit after the Root result or after Reporter has queued its
// update. The Task epoch stays fixed; the fire lineage must fence both queues.
func TestAutomationScheduleEditFencesCommittedFollowups(t *testing.T) {
	for _, reported := range []bool{false, true} {
		t.Run(fmt.Sprintf("reported=%t", reported), func(t *testing.T) {
			ctx := context.Background()
			s, r, a, p := automationFixture(t)
			p.respond = func(llm.GenerationRequest) (llm.GenerationResult, error) {
				return llm.GenerationResult{Text: "reminder result"}, nil
			}
			now := time.Now().UTC()
			id := createAutomation(t, r, now, nil)
			fire := now.AddDate(0, 0, 1)
			r.SetTestAutomationClock(fire)
			if err := r.ScheduleAutomations(ctx, fire); err != nil {
				t.Fatal(err)
			}
			if err := r.admitAutomations(ctx); err != nil {
				t.Fatal(err)
			}
			var root string
			if err := s.db.QueryRow(`SELECT root_run_id FROM automation_fire`).Scan(&root); err != nil {
				t.Fatal(err)
			}
			claim, err := r.Claim(ctx, root)
			if err != nil {
				t.Fatal(err)
			}
			if err = executeTaskRun(t, a, r, ctx, claim); err != nil {
				t.Fatal(err)
			}
			if scalar(t, s, `SELECT count(*) FROM task_update`) != 1 {
				t.Fatal("result not committed")
			}
			if reported {
				if err = r.Report(ctx); err != nil {
					t.Fatal(err)
				}
				if scalar(t, s, `SELECT count(*) FROM main_inbox WHERE run_id IS NULL`) != 1 {
					t.Fatal("report not queued")
				}
			}
			var version int64
			if err = s.db.QueryRow(`SELECT version FROM automation WHERE automation_id=?`, id).Scan(&version); err != nil {
				t.Fatal(err)
			}
			editedFire := fire.AddDate(0, 0, 2)
			result, err := r.ManageAutomation(ctx, "edit", automation.Request{Operation: "update", Ref: id, Version: version, Instruction: "Edited reminder", Type: "at", Spec: editedFire.Format(time.RFC3339), Timezone: "UTC"}, fire)
			if err != nil || result.Error != nil {
				t.Fatal(result, err)
			}
			mustDrain(t, a)
			mustDrain(t, a)
			if scalar(t, s, `SELECT count(*) FROM run WHERE kind='main'`) != 0 || scalar(t, s, `SELECT count(*) FROM event_inbox WHERE consumer='task_reporter'`) != 1 {
				t.Fatal("obsolete update was not consumed without Main")
			}
			if scalar(t, s, `SELECT count(*) FROM main_inbox WHERE run_id IS NOT NULL`) != 0 {
				t.Fatal("obsolete inbox promoted")
			}
			r.SetTestAutomationClock(editedFire)
			mustDrain(t, a)
			mustDrain(t, a)
			if scalar(t, s, `SELECT count(*) FROM run WHERE kind='main' AND status='completed'`) != 1 || scalar(t, s, `SELECT count(*) FROM main_inbox WHERE run_id IS NOT NULL`) != 1 {
				t.Fatal("edited intent did not complete exactly one Main followup")
			}
		})
	}
}

func TestAutomationForgetSkipsRevokedTaskWithoutBlockingActor(t *testing.T) {
	ctx := context.Background()
	s, r, a, p := automationFixture(t)
	m, err := memorysqlite.Open(ctx, filepath.Dir(s.path))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	r.ConfigureMemory(m, memory.Vault{})
	p.respond = func(llm.GenerationRequest) (llm.GenerationResult, error) {
		return llm.GenerationResult{Text: "reminder result"}, nil
	}
	now := time.Now().UTC()
	id := createAutomation(t, r, now, nil)
	first := now.AddDate(0, 0, 1)
	r.SetTestAutomationClock(first)
	mustDrain(t, a)
	var oldTask string
	if err = s.db.QueryRow(`SELECT task_id FROM automation WHERE automation_id=?`, id).Scan(&oldTask); err != nil {
		t.Fatal(err)
	}
	if scalar(t, s, `SELECT count(*) FROM run WHERE task_id=?`, oldTask) != 1 {
		t.Fatal("first recurring occurrence did not finish")
	}
	forget, err := r.ManageMemory(ctx, "forget-health", memory.Mutation{Operation: "forget", Category: "health"})
	if err != nil || forget["accepted"] != true {
		t.Fatal(forget, err)
	}
	p.requests = nil
	fresh := mustSubmit(t, a, "fresh", "Fresh input after Forget")
	second := first.AddDate(0, 0, 1)
	r.SetTestAutomationClock(second)
	mustDrain(t, a)
	mustDrain(t, a)
	if scalar(t, s, `SELECT count(*) FROM automation_fire WHERE state='skipped'`) != 1 || scalar(t, s, `SELECT count(*) FROM event_inbox WHERE consumer='automation_actor'`) != 2 {
		t.Fatal("revoked fire not durably skipped and consumed")
	}
	if scalar(t, s, `SELECT count(*) FROM run WHERE run_id=? AND status='completed'`, fresh.RunID) != 1 {
		t.Fatal("revoked fire blocked fresh Main")
	}
	if scalar(t, s, `SELECT count(*) FROM task WHERE task_id=? AND status='active'`, oldTask) != 1 || scalar(t, s, `SELECT count(*) FROM run WHERE task_id=?`, oldTask) != 1 {
		t.Fatal("Task was cancelled or revoked context revived")
	}
	third := second.AddDate(0, 0, 1)
	result, err := r.ManageAutomation(ctx, "fresh-automation", automation.Request{Operation: "create", Instruction: "Fresh scheduled intent", Type: "at", Spec: third.Format(time.RFC3339), Timezone: "UTC"}, second)
	if err != nil || result.Error != nil {
		t.Fatal(result, err)
	}
	r.SetTestAutomationClock(third)
	mustDrain(t, a)
	if scalar(t, s, `SELECT count(*) FROM run WHERE kind='worker' AND status='completed'`) != 2 || scalar(t, s, `SELECT count(*) FROM automation_fire WHERE state='skipped'`) != 2 || scalar(t, s, `SELECT count(*) FROM automation_fire WHERE state='pending'`) != 0 {
		t.Fatal("fresh Automation blocked or revoked occurrence executed")
	}
	for _, req := range p.requests {
		raw, _ := encoded(req)
		if strings.Contains(raw, "HKG-CDG 2026-12-01 price") {
			t.Fatal("revoked Task context imported", raw)
		}
	}
}

func TestAutomationAdmissionStorageFailureRemainsPending(t *testing.T) {
	ctx := context.Background()
	s, r, a, p := automationFixture(t)
	p.respond = func(llm.GenerationRequest) (llm.GenerationResult, error) {
		return llm.GenerationResult{Text: "reminder result"}, nil
	}
	now := time.Now().UTC()
	createAutomation(t, r, now, nil)
	fire := now.AddDate(0, 0, 1)
	r.SetTestAutomationClock(fire)
	if err := r.ScheduleAutomations(ctx, fire); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`CREATE TRIGGER reject_automation_root BEFORE INSERT ON run WHEN NEW.kind='worker' BEGIN SELECT RAISE(ABORT,'storage fault'); END`); err != nil {
		t.Fatal(err)
	}
	if err := r.admitAutomations(ctx); !errors.Is(err, agent.ErrStorage) {
		t.Fatalf("storage failure swallowed: %v", err)
	}
	if scalar(t, s, `SELECT count(*) FROM automation_fire WHERE state='pending'`) != 1 || scalar(t, s, `SELECT count(*) FROM task`) != 0 || scalar(t, s, `SELECT count(*) FROM event_inbox WHERE consumer='automation_actor'`) != 0 {
		t.Fatal("storage failure partially committed admission")
	}
	if _, err := s.db.Exec(`DROP TRIGGER reject_automation_root`); err != nil {
		t.Fatal(err)
	}
	mustDrain(t, a)
	if scalar(t, s, `SELECT count(*) FROM run WHERE kind='worker' AND status='completed'`) != 1 {
		t.Fatal("recoverable storage failure lost occurrence")
	}
}

func TestAutomationMigrationPreservesPriorAndRollsBack(t *testing.T) {
	ctx := context.Background()
	db := schemaDB(t)
	if e := migrate(ctx, db, migrations[:22]); e != nil {
		t.Fatal(e)
	}
	if _, e := db.Exec(`INSERT INTO agent_config VALUES(1,'prior-automation',NULL,'setup_required',7,1,1)`); e != nil {
		t.Fatal(e)
	}
	failed := append(append([]string{}, migrations[:22]...), automationSQL+"; INVALID SQL")
	if e := migrate(ctx, db, failed); e == nil {
		t.Fatal("fault accepted")
	}
	var version, tables int
	db.QueryRow(`PRAGMA user_version`).Scan(&version)
	db.QueryRow(`SELECT count(*) FROM sqlite_schema WHERE name='automation'`).Scan(&tables)
	if version != 22 || tables != 0 {
		t.Fatal("partial schema migration")
	}
	if e := migrate(ctx, db, migrations); e != nil {
		t.Fatal(e)
	}
	var id string
	var config int
	if e := db.QueryRow(`SELECT agent_id,config_version FROM agent_config`).Scan(&id, &config); e != nil || id != "prior-automation" || config != 7 {
		t.Fatal("prior data lost", e)
	}
}

func TestAutomationCapabilityReductionPreservesChildAdmission(t *testing.T) {
	s, _, a, p := childFixture(t, automation.Definitions()...)
	p.respond = childPolicy
	mustSubmit(t, a, "tree", "delegate normally with Automation registry")
	mustDrain(t, a)
	if scalar(t, s, `SELECT count(*) FROM run WHERE worker_position='child' AND status='completed'`) != 2 {
		t.Fatal("Automation broke recursive Child admission")
	}
	if scalar(t, s, `SELECT count(*) FROM run r JOIN capability_snapshot c ON c.snapshot_id=r.capability_snapshot_id,json_each(c.tools_json) j WHERE (r.kind='main' OR r.worker_position IN ('root','child')) AND json_extract(j.value,'$.name')=?`, automation.ObserveName) != 0 {
		t.Fatal("Root observation authority escaped")
	}
	if scalar(t, s, `SELECT count(*) FROM run r JOIN capability_snapshot c ON c.snapshot_id=r.capability_snapshot_id,json_each(c.tools_json) j WHERE r.kind='worker' AND json_extract(j.value,'$.name')=?`, automation.ManageName) != 0 {
		t.Fatal("Automation control escaped Main")
	}
}
