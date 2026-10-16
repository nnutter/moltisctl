-- Input contract from Shelley v0.1363.920203221:
-- db/schema/012-custom-models.sql and db/query/models.sql.
-- Only connection fields are imported; tuning and display metadata are not.
CREATE TABLE models (
    model_id TEXT PRIMARY KEY,
    display_name TEXT NOT NULL,
    provider_type TEXT NOT NULL,
    endpoint TEXT NOT NULL,
    api_key TEXT NOT NULL,
    model_name TEXT NOT NULL,
    max_tokens INTEGER NOT NULL DEFAULT 200000,
    tags TEXT NOT NULL DEFAULT ''
);
INSERT INTO models (model_id, display_name, provider_type, endpoint, api_key, model_name) VALUES
    ('a', 'Meta chat A', 'openai', 'https://meta.example/v1', 'chat-fixture-key', 'model-a'),
    ('b', 'Meta chat B', 'openai', 'https://meta.example/v1', 'chat-fixture-key', 'model-b'),
    ('c', 'Meta responses', 'openai-responses', 'https://meta.example/v1', 'responses-fixture-key', 'model-r'),
    ('d', 'Other account', 'openai', 'https://meta.example/v1', 'other-fixture-key', 'model-c'),
    ('e', 'Duplicate display entry', 'openai', 'https://meta.example/v1', 'chat-fixture-key', 'model-a');
