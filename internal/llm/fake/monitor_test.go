package fake

import (
	"encoding/json"
	"testing"

	"github.com/z-chenhao/erii/internal/llm"
)

func TestMonitorFixtureRequiresTwoMatchingCurrentFlightSources(t *testing.T) {
	quote := map[string]any{"value": 1750, "route": "HKG-CDG", "flight_date": "2026-12-01", "observed_on": "2026-10-18", "currency": "CNY", "source": "site-b"}
	for _, tc := range []struct {
		field string
		value any
	}{
		{"", nil}, {"value", nil}, {"value", -1}, {"value", 1740}, {"route", "HKG-NRT"},
		{"flight_date", "2026-12-02"}, {"observed_on", "2026-10-17"}, {"currency", "USD"}, {"source", "site-a"},
	} {
		t.Run(tc.field+"="+string(mustJSON(tc.value)), func(t *testing.T) {
			changed := make(map[string]any)
			for k, v := range quote {
				changed[k] = v
			}
			if tc.field != "" {
				changed[tc.field] = tc.value
			}
			r := llm.GenerationRequest{Messages: []llm.Message{
				{Role: "system", Text: "general Worker executing"},
				{Role: "user", Text: "Task anchor: truncated Monitor source: invalid title\nMonitor source: {\"route\":\"HKG-CDG\",\"date\":\"2026-12-01\",\"threshold_cny\":1800}\nAutomation scheduled occurrence: 2026-10-18T00:00:00Z."},
				{Role: "tool", Text: `{"value":1750,"route":"HKG-CDG","flight_date":"2026-12-01","observed_on":"2026-10-18","currency":"CNY","source":"site-a"}`},
				{Role: "tool", Text: string(mustJSON(changed))},
			}}
			result := monitorReply(r)
			if tc.field == "" {
				if len(result.ToolCalls) != 1 || result.ToolCalls[0].Name != "builtin__automation__observe" || result.ToolCalls[0].Arguments != `{"value":1750}` {
					t.Fatal(result)
				}
			} else if len(result.ToolCalls) != 0 || result.Text == "" {
				t.Fatal("unverified price observed", result)
			}
		})
	}
}

func mustJSON(value any) []byte {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return raw
}
