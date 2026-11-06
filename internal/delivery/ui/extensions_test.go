package ui

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTypedFormValidationAndRestore(t *testing.T) {
	c := Component{Type: "openintelligentui", Name: "steward.form", Props: json.RawMessage(`{"title":"Trip","fields":[{"name":"date","label":"Date","type":"date","required":true},{"name":"count","label":"Travelers","type":"number","min":1,"max":5},{"name":"mode","label":"Mode","type":"select","options":[{"value":"train","label":"Train"}]},{"name":"extras","label":"Extras","type":"multiselect","options":[{"value":"meal","label":"Meal"},{"value":"bag","label":"Bag"}]},{"name":"confirm","label":"Confirm","type":"boolean","required":true}]}`)}
	if !validNative(c) {
		t.Fatal("valid form rejected")
	}
	values := map[string]string{"date": "2026-11-07", "count": "2", "mode": "train", "extras": `["bag","meal"]`, "confirm": "true"}
	parts, err := Answer(c, values)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(parts[4].Text, `["meal","bag"]`) {
		t.Fatal("selection identity depends on click order", parts)
	}
	if err = Restore(&c, parts); err != nil || !c.Submitted || !strings.Contains(c.Response, "2026-11-07") {
		t.Fatal(c, err)
	}
	for field, value := range map[string]string{"date": "2026-02-30", "count": "NaN", "mode": "air", "extras": `["meal","meal"]`, "confirm": "false"} {
		bad := map[string]string{}
		for k, v := range values {
			bad[k] = v
		}
		bad[field] = value
		if _, err := Answer(c, bad); err == nil {
			t.Fatal("invalid field accepted", field)
		}
	}
	if html := nativeHTML(c); !strings.Contains(html, `type="date"`) || !strings.Contains(html, `name="field.extras[]"`) {
		t.Fatal("native fallback lost field semantics", html)
	}
}
