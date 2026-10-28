package sqlite

import inspectionsqlite "github.com/z-chenhao/erii/internal/observability/sqlite"

// Inspector exposes the read-only projection, not the Runtime writer pool.
func (s *Store) Inspector() *inspectionsqlite.Inspector { return s.inspector }
