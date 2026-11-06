-- Only broker references and locally authorized bindings live here. OAuth
-- tokens remain in Composio; the project key remains in SecretStore.
CREATE TABLE connected_apps (
 singleton INTEGER PRIMARY KEY CHECK(singleton=1),
 revision INTEGER NOT NULL CHECK(revision>0),
 document_json TEXT NOT NULL CHECK(json_valid(document_json))
) STRICT;
INSERT INTO connected_apps VALUES(1,1,json_object(
 'key_ref','', 'user_id','steward_'||lower(hex(randomblob(16))),
 'connections',json('[]')));
