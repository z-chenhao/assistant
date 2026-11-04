package sqlite

import (
	"context"
	"database/sql"
	"slices"
	"strings"

	"github.com/z-chenhao/assistant/internal/database"
)

func marks(n int) string { return strings.TrimSuffix(strings.Repeat("?,", n), ",") }

func (r *Reader) memoryExecutions(ctx context.Context, owner string, sources map[string][]source, records []database.Record) error {
	indices := map[string][]int{}
	for i, record := range records {
		for _, source := range sources[record.Ref] {
			if source.Kind != "feedback" || !strings.HasPrefix(source.Ref, "feedback:") {
				continue
			}
			id := strings.TrimPrefix(source.Ref, "feedback:")
			if database.ValidRef("run:" + id) {
				indices[id] = append(indices[id], i)
			}
		}
	}
	if len(indices) == 0 {
		return nil
	}
	args := []any{owner}
	for id := range indices {
		args = append(args, id)
	}
	rows, err := r.state.QueryContext(ctx, `SELECT i.invocation_id,i.run_id FROM tool_invocation i JOIN run r USING(run_id) WHERE r.agent_id=? AND i.tool_name='builtin__memory_manage' AND i.invocation_id IN (`+marks(len(indices))+")", args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var invocation, run string
		if err := rows.Scan(&invocation, &run); err != nil {
			return err
		}
		for _, index := range indices[invocation] {
			link(&records[index], "Source execution", "run:"+run)
		}
	}
	return rows.Err()
}

func (r *Reader) messageExecutions(ctx context.Context, owner string, records []database.Record) error {
	if len(records) == 0 {
		return nil
	}
	indices, args := map[string]int{}, []any{owner}
	for i, record := range records {
		id := strings.TrimPrefix(record.Ref, "conversation:")
		indices[id] = i
		args = append(args, id)
	}
	// Accepted inputs and committed replies are causal facts. Never infer an
	// execution from message text, source_event_id or a temporal coincidence.
	query := `SELECT i.message_id,i.run_id FROM main_input i JOIN run r USING(run_id) WHERE r.agent_id=? AND i.message_id IN (` + marks(len(records)) + `)
 UNION ALL SELECT i.message_id,i.run_id FROM main_reply i JOIN run r USING(run_id) WHERE r.agent_id=? AND i.message_id IN (` + marks(len(records)) + `)`
	args = append(args, args...)
	rows, err := r.state.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var message, run string
		if err = rows.Scan(&message, &run); err != nil {
			return err
		}
		link(&records[indices[message]], "Execution", "run:"+run)
	}
	return rows.Err()
}

// Resolve every candidate against its actual owning database, including Memory
// lineage and sources. A stored reference is not permission to inspect its target.
func (r *Reader) filterLinks(ctx context.Context, owner string, records []database.Record) error {
	groups := map[string]map[string]bool{}
	for _, record := range records {
		for _, link := range record.Links {
			if !database.ValidRef(link.Ref) {
				continue
			}
			kind := database.Collection(link.Ref)
			if strings.HasPrefix(link.Ref, "task:") {
				kind = "tasks"
			}
			if groups[kind] == nil {
				groups[kind] = map[string]bool{}
			}
			groups[kind][link.Ref] = true
		}
	}
	visible := map[string]bool{}
	for kind, refs := range groups {
		var db *sql.DB
		var query, prefix string
		args := []any{owner}
		switch kind {
		case "executions":
			db, prefix, query = r.state, "run:", `SELECT run_id FROM run WHERE agent_id=? AND run_id IN (`
		case "messages":
			db, prefix, query = r.history, "conversation:", `SELECT m.message_id FROM conversation_message m JOIN conversation_scope s USING(conversation_id) WHERE s.user_id='local' AND s.agent_id=? AND m.message_id IN (`
		case "memories":
			db, prefix, query = r.memory, "memory:", `SELECT memory_id FROM memory_record WHERE user_id='local' AND ((scope_type='user' AND scope_id='local') OR (scope_type='agent' AND scope_id=?)) AND memory_id IN (`
		case "artifacts":
			db, query = r.state, `SELECT artifact_ref FROM artifact WHERE owner_user_id='local' AND owner_agent_id=? AND artifact_ref IN (`
		case "tasks":
			db, query = r.state, `SELECT task_ref FROM task WHERE agent_id=? AND task_ref IN (`
		default:
			continue
		}
		for ref := range refs {
			args = append(args, strings.TrimPrefix(ref, prefix))
		}
		rows, err := db.QueryContext(ctx, query+marks(len(refs))+")", args...)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			visible[prefix+id] = true
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
	}
	for i := range records {
		kept, seen := []database.Link{}, map[database.Link]bool{}
		for _, link := range records[i].Links {
			if visible[link.Ref] && !seen[link] {
				kept = append(kept, link)
				seen[link] = true
			}
		}
		slices.SortFunc(kept, func(a, b database.Link) int {
			if n := strings.Compare(a.Relation, b.Relation); n != 0 {
				return n
			}
			return strings.Compare(a.Ref, b.Ref)
		})
		records[i].Links = kept
	}
	return nil
}
