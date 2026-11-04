package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/z-chenhao/assistant/internal/database"
)

// Every identifier and selected column is fixed here. Body values, ciphertext,
// credentials, request keys, host paths and frozen payloads are never selected.
func (r *Reader) projection(kind, owner string) projection {
	switch kind {
	case "executions":
		return projection{r.state,
			`r.run_id,r.created_at,r.kind,coalesce(r.worker_position,''),r.status,r.depth,coalesce(r.task_epoch,0),r.version,r.started_at,r.ended_at,coalesce(t.task_ref,''),coalesce(r.parent_run_id,''),coalesce(r.root_run_id,'')`,
			` FROM run r LEFT JOIN task t ON t.task_id=r.task_id WHERE r.agent_id=?`, "r.run_id", "r.created_at", []any{owner}, scanExecution}
	case "messages":
		return projection{r.history,
			`m.message_id,m.created_at,m.sequence,m.role,m.source,json_array_length(m.content_json),
 (SELECT coalesce(json_group_array(ref),'[]') FROM (SELECT DISTINCT json_extract(j.value,'$.artifact_ref') ref FROM json_each(m.content_json) j WHERE j.type='object' AND json_extract(j.value,'$.artifact_ref') IS NOT NULL AND length(CAST(json_extract(j.value,'$.artifact_ref') AS BLOB))<=256 ORDER BY ref LIMIT 16))`,
			` FROM conversation_message m JOIN conversation_scope s USING(conversation_id) WHERE s.user_id='local' AND s.agent_id=?`, "m.message_id", "m.sequence", []any{owner}, scanMessage}
	case "memories":
		return projection{r.memory,
			`m.memory_id,m.created_at,m.tier,m.scope_type,m.version,m.deleted_at,substr(lower(m.content),1,7)='sealed:',json_array_length(m.source_refs),coalesce(m.supersedes_memory_id,''),coalesce((SELECT n.memory_id FROM memory_record n WHERE n.supersedes_memory_id=m.memory_id),''),
 (SELECT coalesce(json_group_array(json_object('kind',kind,'ref',ref)),'[]') FROM (SELECT DISTINCT json_extract(j.value,'$.source_type') kind,json_extract(j.value,'$.source_ref') ref FROM json_each(m.source_refs) j WHERE j.type='object' AND json_extract(j.value,'$.source_type') IN ('conversation','task','run','feedback') AND length(CAST(json_extract(j.value,'$.source_ref') AS BLOB))<=256 ORDER BY kind,ref LIMIT 16))`,
			` FROM memory_record m WHERE m.user_id='local' AND ((m.scope_type='user' AND m.scope_id='local') OR (m.scope_type='agent' AND m.scope_id=?))`, "m.memory_id", "m.created_at", []any{owner}, scanMemory}
	case "artifacts":
		return projection{r.state,
			`a.artifact_ref,a.created_at,a.name,a.media_type,a.byte_size,a.content_hash,a.visibility,a.version,a.expires_at,a.deleted_at,coalesce(t.task_ref,''),coalesce(a.run_id,''),(SELECT count(*) FROM artifact_reference x WHERE x.artifact_ref=a.artifact_ref),
 (SELECT coalesce(json_group_array(json_object('relation',relation,'ref',ref)),'[]') FROM (
 SELECT CASE x.holder_kind WHEN 'conversation' THEN 'Attached to message' WHEN 'task' THEN 'Used by task' ELSE 'Used by execution' END relation,
 CASE x.holder_kind WHEN 'conversation' THEN 'conversation:'||x.holder_id WHEN 'run' THEN 'run:'||x.holder_id WHEN 'task' THEN ht.task_ref WHEN 'tool' THEN 'run:'||hi.run_id END ref
 FROM artifact_reference x LEFT JOIN task ht ON x.holder_kind='task' AND ht.task_id=x.holder_id LEFT JOIN tool_invocation hi ON x.holder_kind='tool' AND hi.invocation_id=x.holder_id
 WHERE x.artifact_ref=a.artifact_ref AND x.holder_kind IN ('conversation','task','run','tool') ORDER BY x.holder_kind,x.holder_id LIMIT 16))`,
			` FROM artifact a LEFT JOIN task t ON t.task_id=a.task_id WHERE a.owner_user_id='local' AND a.owner_agent_id=?`, "a.artifact_ref", "a.created_at", []any{owner}, scanArtifact}
	}
	return projection{}
}

func stamp(at int64) string { return time.UnixMicro(at).UTC().Format(time.RFC3339Nano) }
func optionalStamp(at sql.NullInt64) string {
	if !at.Valid {
		return "—"
	}
	return stamp(at.Int64)
}
func field(name string, value any) database.Field {
	return database.Field{Name: name, Value: fmt.Sprint(value)}
}
func record(ref, status string, at int64, fields ...database.Field) database.Record {
	return database.Record{Ref: ref, Status: status, CreatedAt: stamp(at), Fields: fields, Links: []database.Link{}}
}
func link(v *database.Record, relation, ref string) {
	if database.ValidRef(ref) {
		v.Links = append(v.Links, database.Link{Relation: relation, Ref: ref})
	}
}

func scanExecution(s scanner) (row, error) {
	var v row
	var role, position, status, task, parent, root string
	var depth, epoch, version int64
	var started, ended sql.NullInt64
	err := s.Scan(&v.id, &v.at, &role, &position, &status, &depth, &epoch, &version, &started, &ended, &task, &parent, &root)
	if role == "worker" {
		role = position
	}
	v.Record = record("run:"+v.id, status, v.at, field("Role", role), field("Depth", depth), field("Task epoch", epoch), field("Version", version), field("Started at", optionalStamp(started)), field("Ended at", optionalStamp(ended)))
	link(&v.Record, "Task", task)
	link(&v.Record, "Parent execution", "run:"+parent)
	if root != v.id {
		link(&v.Record, "Root execution", "run:"+root)
	}
	return v, err
}

func scanMessage(s scanner) (row, error) {
	var v row
	var created, parts int64
	var role, source, attachments string
	err := s.Scan(&v.id, &created, &v.at, &role, &source, &parts, &attachments)
	v.Record = record("conversation:"+v.id, "recorded", created, field("Role", role), field("Source", source), field("Sequence", v.at), field("Content parts", parts))
	var refs []string
	if err == nil {
		err = json.Unmarshal([]byte(attachments), &refs)
	}
	for _, ref := range refs {
		link(&v.Record, "Attachment", ref)
	}
	return v, err
}

func scanMemory(s scanner) (row, error) {
	var v row
	var tier, scope, previous, next, sources string
	var version, count int64
	var sealed bool
	var deleted sql.NullInt64
	err := s.Scan(&v.id, &v.at, &tier, &scope, &version, &deleted, &sealed, &count, &previous, &next, &sources)
	status, representation := "current", "legacy plaintext"
	if sealed {
		representation = "encrypted"
	}
	if next != "" {
		status = "superseded"
	}
	if deleted.Valid {
		status = "deleted"
	}
	v.Record = record("memory:"+v.id, status, v.at, field("Tier", tier), field("Scope", scope), field("Storage form", representation), field("Source count", count), field("Version", version), field("Deleted at", optionalStamp(deleted)))
	link(&v.Record, "Replaces", "memory:"+previous)
	link(&v.Record, "Replaced by", "memory:"+next)
	if err == nil {
		err = json.Unmarshal([]byte(sources), &v.sources)
	}
	for _, source := range v.sources {
		if source.Kind == "conversation" && database.Collection(source.Ref) == "messages" {
			link(&v.Record, "Source message", source.Ref)
		} else if source.Kind == "task" && len(source.Ref) >= 5 && source.Ref[:5] == "task:" {
			link(&v.Record, "Source task", source.Ref)
		} else if source.Kind == "run" && database.Collection(source.Ref) == "executions" {
			link(&v.Record, "Source execution", source.Ref)
		}
	}
	return v, err
}

func scanArtifact(s scanner) (row, error) {
	var v row
	var name, media, hash, visibility, task, run, referencesJSON string
	var size, version, expires, references int64
	var deleted sql.NullInt64
	err := s.Scan(&v.id, &v.at, &name, &media, &size, &hash, &visibility, &version, &expires, &deleted, &task, &run, &references, &referencesJSON)
	status := "stored"
	if deleted.Valid {
		status = "deleted"
	}
	v.Record = record(v.id, status, v.at, field("Name", name), field("Media type", media), field("Bytes", size), field("SHA-256", hash), field("Visibility", visibility), field("Version", version), field("Retention deadline", stamp(expires)), field("Deleted at", optionalStamp(deleted)), field("Reference count", references))
	link(&v.Record, "Task", task)
	link(&v.Record, "Produced by", "run:"+run)
	var holders []database.Link
	if err == nil {
		err = json.Unmarshal([]byte(referencesJSON), &holders)
	}
	for _, holder := range holders {
		link(&v.Record, holder.Relation, holder.Ref)
	}
	return v, err
}
