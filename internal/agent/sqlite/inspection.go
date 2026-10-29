package sqlite

import (
	inspectionsqlite "github.com/z-chenhao/erii/internal/observability/sqlite"
	tasksqlite "github.com/z-chenhao/erii/internal/task/sqlite"
)

// Inspector exposes the read-only projection, not the Runtime writer pool.
func (s *Store) Inspector() *inspectionsqlite.Inspector { return s.inspector }

func (s *Store) DispatchRecorder() *inspectionsqlite.Recorder { return s.dispatches }

func (s *Store) TaskReader() *tasksqlite.Reader { return s.taskReader }
