package sqlite

import (
	"context"
	"database/sql"

	"github.com/z-chenhao/assistant/internal/observer/github"
	"github.com/z-chenhao/assistant/internal/task"
)

// The successful scan and its decision commit together. A scan begun before
// this generation can still retain evidence but cannot authorize this rule.
func recordGitHubRuleDecision(ctx context.Context, tx *sql.Tx, scope string, ticket github.Ticket, sequence int64, kind string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO github_rule_decision(observation_sequence,task_id,mode,event_kind,matched)
 SELECT ?,b.task_id,b.rule_mode,b.rule_event_kind,b.rule_event_kind=?
 FROM github_task_binding b JOIN task t USING(task_id)
 WHERE b.scope=? AND b.generation=? AND b.privacy_epoch=? AND b.enabled=1 AND b.rule_event_kind!=''
 AND b.task_epoch=t.task_epoch AND t.status IN('active','completed') AND t.explicit_end_epoch<t.task_epoch
 AND t.agent_id=(SELECT agent_id FROM agent_config WHERE singleton=1)
 AND t.conversation_id=t.agent_id AND t.created_at>coalesce((SELECT updated_at FROM memory_privacy WHERE agent_id=t.agent_id),0)`,
		sequence, kind, scope, ticket.BindingGeneration, ticket.Epoch)
	return err
}

func (s *Store) GitHubRuleDecisions(ctx context.Context, cfg github.Config, before int64) ([]github.RuleDecision, error) {
	if cfg.Validate() != nil || before < 0 {
		return nil, task.ErrInvalid
	}
	rows, err := s.db.QueryContext(ctx, `SELECT o.sequence,o.event_key,o.kind,d.event_kind,t.task_ref,o.binding_generation,d.mode,d.matched
 FROM github_observation o JOIN github_rule_decision d ON d.observation_sequence=o.sequence
 JOIN github_watch w ON w.scope=o.scope JOIN task t ON t.task_id=d.task_id
 WHERE o.scope=? AND o.intent_privacy_epoch=`+githubEpoch+` AND w.intent_privacy_epoch=o.intent_privacy_epoch
 AND w.enabled=1 AND w.failure_code NOT IN ('REPOSITORY_IDENTITY_CHANGED','CREDENTIAL_REJECTED')
 AND t.agent_id=(SELECT agent_id FROM agent_config WHERE singleton=1)
 AND t.conversation_id=t.agent_id AND t.created_at>coalesce((SELECT updated_at FROM memory_privacy WHERE agent_id=t.agent_id),0)
 AND (?=0 OR o.sequence<?) ORDER BY o.sequence DESC LIMIT 50`, cfg.Scope(), before, before)
	if err != nil {
		return nil, githubStorageError(err)
	}
	defer rows.Close()
	decisions := []github.RuleDecision{}
	for rows.Next() {
		var d github.RuleDecision
		if err = rows.Scan(&d.Sequence, &d.EventKey, &d.Kind, &d.ExpectedKind, &d.TaskRef, &d.Generation, &d.Mode, &d.Matched); err != nil {
			return nil, githubStorageError(err)
		}
		d.Reason = "event_kind_mismatch"
		if d.Matched {
			d.Reason = "event_kind_matched"
		}
		decisions = append(decisions, d)
	}
	return decisions, githubStorageError(rows.Err())
}
