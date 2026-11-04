-- Existing 039 question rows and their frozen projections retain their shape.
ALTER TABLE feishu_task_question ADD COLUMN review_url TEXT NOT NULL DEFAULT '';
