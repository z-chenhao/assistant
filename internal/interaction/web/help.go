package web

// Help is plain, localized interface guidance. It never accepts HTML. Its native
// details fallback needs a flow-content parent; a paragraph would split the DOM.
type helpView struct{ ID, Label, Text string }

func help(id, label, text string) helpView { return helpView{ID: id, Label: label, Text: text} }
