-- Read-only inspection indexes. No frozen requests, receipts or snapshots change.
CREATE INDEX run_inspection_created ON run(agent_id,created_at DESC,run_id DESC);
CREATE INDEX run_inspection_status ON run(agent_id,status,created_at DESC,run_id DESC);
CREATE INDEX run_inspection_task ON run(task_id,created_at,run_id);
CREATE INDEX task_request_reference ON task_request(json_extract(receipt_json,'$.taskRef'),invocation_id);
