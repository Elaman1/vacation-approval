CREATE TABLE IF NOT EXISTS vacation.decisions
(
    event_id UInt64,
    request_id Int64,
    employee_id Int64,
    days Int32,
    decision LowCardinality(String),
    occurred_at DateTime64(3, 'UTC'),
    ingested_at DateTime64(3, 'UTC') DEFAULT now64(3)
)
ENGINE = ReplacingMergeTree(ingested_at)
ORDER BY (event_id);
