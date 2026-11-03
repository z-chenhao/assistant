-- A Task question has at most one interactive delivery, with the channel
-- binding version authorized when Main saved it.
CREATE TABLE feishu_task_question (
 feedback_ref TEXT PRIMARY KEY REFERENCES task_feedback(feedback_ref),
 message_id TEXT NOT NULL UNIQUE REFERENCES message_route(message_id),
 binding_version INTEGER NOT NULL CHECK(binding_version>=1)
) STRICT;
CREATE TRIGGER feishu_task_question_immutable BEFORE UPDATE ON feishu_task_question
 BEGIN SELECT RAISE(ABORT,'immutable Feishu Task question'); END;
CREATE TRIGGER feishu_task_question_retained BEFORE DELETE ON feishu_task_question
 BEGIN SELECT RAISE(ABORT,'Feishu Task question retained'); END;
CREATE TRIGGER feishu_task_question_no_replace BEFORE INSERT ON feishu_task_question
 WHEN EXISTS(SELECT 1 FROM feishu_task_question WHERE feedback_ref=NEW.feedback_ref OR message_id=NEW.message_id)
 BEGIN SELECT RAISE(ABORT,'Feishu Task question identity exists'); END;
