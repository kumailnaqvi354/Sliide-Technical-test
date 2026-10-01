-- +goose Up
-- What editors have asked for, as opposed to articles.disabled, which is what
-- the consumer has applied. The two are compared to tell whether a request is
-- still in flight. The consumer never reads or writes this table.
CREATE TABLE article_status_requests (
    -- Also sent as the queue message's trace_id.
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    article_id   UUID NOT NULL REFERENCES articles (id) ON DELETE CASCADE,
    action       TEXT NOT NULL CHECK (action IN ('disable', 'enable')),
    requested_at TIMESTAMPTZ NOT NULL
);

-- Every read wants the latest request for an article.
CREATE INDEX article_status_requests_latest_idx
    ON article_status_requests (article_id, requested_at DESC);

-- +goose Down
DROP TABLE article_status_requests;
