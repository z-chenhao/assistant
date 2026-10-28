-- Native Computer tools share the durable execution/Computer ledger without a
-- JavaScript wrapper. Historical frozen requests and snapshots are unchanged.
DROP TRIGGER IF EXISTS code_wrapper_terminal;
CREATE TRIGGER code_wrapper_terminal AFTER UPDATE OF status ON tool_invocation WHEN NEW.tool_name IN ('builtin__codemode__execute','builtin__codemode_execute','builtin__read','builtin__bash','builtin__edit','builtin__write') AND NEW.status IN ('succeeded','failed','cancelled','unknown') BEGIN
 UPDATE code_execution_state SET state=CASE WHEN NEW.status='succeeded' THEN 'completed' WHEN NEW.status='cancelled' THEN 'cancelled' ELSE 'failed' END,processed_wake_seq=wake_seq,result_ref=NEW.result_ref,error_json=NEW.error_json,claim_token=NULL,owner_run_token=NULL,claim_expires_at=0,updated_at=NEW.updated_at,version=version+1 WHERE execution_id=NEW.invocation_id;
END;
