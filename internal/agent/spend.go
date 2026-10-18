package agent

import (
	"errors"
	"regexp"

	"go.yaml.in/yaml/v3"
)

// SpendPolicy is private Host configuration. Amounts are millionths of the
// named currency; rates are millionths per million tokens, never floats.
type SpendPolicy struct {
	Accounts []SpendAccount `yaml:"accounts" json:"accounts"`
}
type SpendAccount struct {
	Account  string       `yaml:"account" json:"account"`
	Currency string       `yaml:"currency" json:"currency"`
	Boundary int64        `yaml:"boundary_micros" json:"boundary_micros"`
	Prices   []ModelPrice `yaml:"prices" json:"prices"`
}
type ModelPrice struct {
	Model      string `yaml:"model" json:"model"`
	Version    string `yaml:"version" json:"version"`
	Input      int64  `yaml:"input_micros_per_million" json:"input_micros_per_million"`
	Output     int64  `yaml:"output_micros_per_million" json:"output_micros_per_million"`
	CacheRead  int64  `yaml:"cache_read_micros_per_million" json:"cache_read_micros_per_million"`
	CacheWrite int64  `yaml:"cache_write_micros_per_million" json:"cache_write_micros_per_million"`
}

func (a *SpendAccount) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return errors.New("invalid account monetary policy")
	}
	keys := map[string]bool{}
	for i := 0; i < len(n.Content); i += 2 {
		key := n.Content[i].Value
		if keys[key] || (key != "account" && key != "currency" && key != "boundary_micros" && key != "prices") {
			return errors.New("unknown or duplicate monetary policy field")
		}
		keys[key] = true
		if key == "boundary_micros" && n.Content[i+1].Tag != "!!int" {
			return errors.New("monetary boundary must be an integer")
		}
	}
	type plain SpendAccount
	return n.Decode((*plain)(a))
}

// An omitted rate is unknown, not a zero price. Zero is accepted only when the
// operator writes it explicitly (including local/subscription model pricing).
func (p *ModelPrice) UnmarshalYAML(n *yaml.Node) error {
	keys := map[string]bool{"model": false, "version": false, "input_micros_per_million": false, "output_micros_per_million": false, "cache_read_micros_per_million": false, "cache_write_micros_per_million": false}
	if n.Kind != yaml.MappingNode {
		return errors.New("invalid monetary price")
	}
	for i := 0; i < len(n.Content); i += 2 {
		key := n.Content[i].Value
		if present, ok := keys[key]; !ok || present {
			return errors.New("unknown or duplicate monetary price field")
		}
		keys[key] = true
		if key != "model" && key != "version" && n.Content[i+1].Tag != "!!int" {
			return errors.New("monetary rates must be integers")
		}
	}
	for _, present := range keys {
		if !present {
			return errors.New("monetary price requires every rate explicitly")
		}
	}
	type plain ModelPrice
	return n.Decode((*plain)(p))
}

func (p SpendPolicy) Validate() error {
	seen := map[string]bool{}
	for _, a := range p.Accounts {
		if !regexp.MustCompile(`^(openai_account|openai_compatible):[a-zA-Z0-9-]{1,64}$`).MatchString(a.Account) || seen[a.Account] || !regexp.MustCompile(`^[A-Z]{3}$`).MatchString(a.Currency) || a.Boundary < 0 || a.Boundary > 1e15 {
			return errors.New("invalid account monetary policy")
		}
		seen[a.Account] = true
		models := map[string]bool{}
		for _, v := range a.Prices {
			if v.Model == "" || len(v.Model) > 256 || !regexp.MustCompile(`^[a-zA-Z0-9._-]{1,64}$`).MatchString(v.Version) || models[v.Model] {
				return errors.New("invalid or duplicate model pricing metadata")
			}
			models[v.Model] = true
			for _, rate := range []int64{v.Input, v.Output, v.CacheRead, v.CacheWrite} {
				if rate < 0 || rate > 1e12 {
					return errors.New("invalid monetary rate")
				}
			}
		}
	}
	return nil
}
