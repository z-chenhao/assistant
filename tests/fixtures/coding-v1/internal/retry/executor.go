package retry

type Provider interface{ Execute() string }
type Ledger interface{ Lookup() string }

// Execute contains the audit fixture's known defect: the external operation
// runs before checking the ledger, so a retry can duplicate the effect.
func Execute(provider Provider, ledger Ledger) string {
	value := provider.Execute()
	if previous := ledger.Lookup(); previous != "" {
		return previous
	}
	return value
}
