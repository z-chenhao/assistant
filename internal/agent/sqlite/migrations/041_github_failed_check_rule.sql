ALTER TABLE github_task_binding ADD COLUMN rule_check_conclusion TEXT NOT NULL DEFAULT ''
 CHECK(rule_check_conclusion='' OR (rule_event_kind='check_run' AND rule_check_conclusion='failure'));
ALTER TABLE github_rule_decision ADD COLUMN expected_check_conclusion TEXT NOT NULL DEFAULT ''
 CHECK(expected_check_conclusion='' OR (event_kind='check_run' AND expected_check_conclusion='failure'));
ALTER TABLE github_rule_decision ADD COLUMN actual_check_conclusion TEXT NOT NULL DEFAULT ''
 CHECK(actual_check_conclusion IN ('','action_required','cancelled','failure','neutral','success','skipped','stale','timed_out','startup_failure','unknown'));
