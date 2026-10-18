package migration

// The initial fixture persists legacy worker jobs in a JSON state file.
// Implement a one-way rewrite to Run tree state; Down is used only in rehearsal.
func Up(path string) error   { return nil } // MIGRATION_UP
func Down(path string) error { return nil } // MIGRATION_DOWN
