CREATE TABLE employees
(
    id               BIGSERIAL PRIMARY KEY,
    keycloak_user_id TEXT    NOT NULL UNIQUE,
    balance_days     INTEGER NOT NULL DEFAULT 0,

    CONSTRAINT employees_balance_check
        CHECK (balance_days >= 0)
);

CREATE TABLE vacation_requests
(
    id                 BIGSERIAL PRIMARY KEY,
    employee_id        BIGINT      NOT NULL REFERENCES employees (id),

    start_date         DATE        NOT NULL,
    end_date           DATE        NOT NULL,
    days               INTEGER     NOT NULL,

    status             TEXT        NOT NULL DEFAULT 'pending_balance',
    camunda_process_id TEXT UNIQUE,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT vacation_dates_check
        CHECK (end_date >= start_date),

    CONSTRAINT vacation_days_check
        CHECK (days > 0),

    CONSTRAINT vacation_status_check
        CHECK (status IN (
                          'pending_balance',
                          'pending_manager',
                          'auto_approved',
                          'approved',
                          'rejected',
                          'needs_hr_review'
            ))
);
