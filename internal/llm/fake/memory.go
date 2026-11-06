package fake

import (
	"encoding/json"
	"strings"

	"github.com/z-chenhao/assistant/internal/llm"
	"github.com/z-chenhao/assistant/internal/memory"
)

// Explicit, finite transport fixtures. This is not the production Curator and
// does not establish any verdict about a real model's semantic quality.
func memoryReply(req llm.GenerationRequest) llm.GenerationResult {
	var in memory.CurationInput
	if len(req.Messages) != 2 || json.Unmarshal([]byte(req.Messages[1].Text), &in) != nil {
		return llm.GenerationResult{Text: `{"facts":[]}`}
	}
	text := strings.TrimSpace(in.Observation.Text)
	fixtures := map[string]memory.Candidate{
		"My health report: I have a diagnosed kidney stone condition":   {Facet: "health.kidney_stones", Category: "health", Content: "The user reports a diagnosed kidney stone condition."},
		"I might explore independent development, but have not decided": {Facet: "goals.independent_development", Category: "goals", Content: "The user is considering independent development, without a decision.", Tentative: true},
		"I am practicing English for travel":                            {Facet: "learning.english", Category: "learning", Content: "The user is practicing English for travel."},
		"我喜欢无糖绿茶":                                                       {Facet: "drinks.preference", Category: "preferences", Content: "用户偏好无糖绿茶"},
		"I prefer green tea without sugar":                              {Facet: "drinks.preference", Category: "preferences", Content: "用户偏好无糖绿茶"},
		"我更喜欢不加糖的绿茶":                                                    {Facet: "drinks.preference", Category: "preferences", Content: "用户偏好无糖绿茶"},
		"我现在更喜欢红茶":                                                      {Facet: "drinks.preference", Category: "preferences", Content: "用户偏好红茶"},
		"我不再偏好红茶":                                                       {Facet: "drinks.preference", Category: "preferences"},
	}
	p := memory.Proposal{Facts: []memory.Candidate{}}
	if c, ok := fixtures[text]; ok {
		c.Operation = "add"
		c.Quote = text
		for _, f := range in.Existing {
			if f.Facet == c.Facet {
				c.Target = f.Ref
				c.Operation = "support"
				if c.Content != f.Content {
					c.Operation = "replace"
				}
				if c.Content == "" {
					c.Operation = "retract"
				}
				break
			}
		}
		if c.Operation == "support" || c.Operation == "retract" {
			c.Content = ""
		}
		p.Facts = append(p.Facts, c)
	}
	raw, _ := json.Marshal(p)
	return llm.GenerationResult{Text: string(raw)}
}
