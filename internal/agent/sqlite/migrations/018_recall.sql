CREATE TABLE run_recall (
 run_id TEXT NOT NULL REFERENCES run(run_id), step_index INTEGER NOT NULL,
 embedding_id TEXT NOT NULL, query TEXT NOT NULL, hits_json TEXT NOT NULL CHECK(json_valid(hits_json)),
 created_at INTEGER NOT NULL, PRIMARY KEY(run_id,step_index)
) STRICT;
CREATE TRIGGER run_recall_immutable BEFORE UPDATE ON run_recall BEGIN SELECT RAISE(ABORT,'immutable recall'); END;
CREATE TRIGGER run_recall_no_delete BEFORE DELETE ON run_recall BEGIN SELECT RAISE(ABORT,'immutable recall'); END;
CREATE TRIGGER run_recall_no_replace BEFORE INSERT ON run_recall WHEN EXISTS(SELECT 1 FROM run_recall WHERE run_id=new.run_id AND step_index=new.step_index) BEGIN SELECT RAISE(ABORT,'immutable recall'); END;
