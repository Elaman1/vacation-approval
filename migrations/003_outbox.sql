-- Written atomically with the final vacation decision.
-- Existing historical decisions are intentionally NOT backfilled.
CREATE TABLE IF NOT EXISTS vacation_outbox (
    id BIGSERIAL PRIMARY KEY,
    request_id BIGINT NOT NULL UNIQUE REFERENCES vacation_requests(id),
    decision TEXT NOT NULL CHECK (decision IN ('auto_approved', 'approved', 'rejected', 'needs_hr_review')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS vacation_outbox_pending_idx
    ON vacation_outbox (id) WHERE published_at IS NULL;
