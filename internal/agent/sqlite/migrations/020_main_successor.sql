-- A revoked Main retains its owner until stop settlement. Fresh ingress owns
-- a separate queued Run; scheduling and Claim wait for all revoked owners.
-- Receipts, input ownership and frozen snapshots remain immutable.
DROP INDEX one_main;
CREATE UNIQUE INDEX one_main ON run(agent_id)
 WHERE kind='main' AND desired_state='running' AND status IN ('queued','running','waiting');
