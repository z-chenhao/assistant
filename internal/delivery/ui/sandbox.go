package ui

import "encoding/json"

// SandboxedUI preserves generateSandboxedUi's ordered parameter contract.
// Delivery freezes completed activity content; it is not executable Host code.
type SandboxedUI struct {
	InitialHeight       float64  `json:"initialHeight"`
	PlaceholderMessages []string `json:"placeholderMessages"`
	CSS                 string   `json:"css"`
	HTML                string   `json:"html"`
	JSFunctions         string   `json:"jsFunctions"`
	JSExpressions       []string `json:"jsExpressions"`
}

func Sandbox(w Widget) (SandboxedUI, bool) {
	var s SandboxedUI
	if len(w.Components) != 1 || w.Components[0].Name != "generateSandboxedUi" || !validNative(w.Components[0]) {
		return s, false
	}
	err := json.Unmarshal(w.Components[0].Props, &s)
	return s, err == nil
}
