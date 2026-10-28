-- The initial 001_init.sql already created vacation_requests.
-- One extra status covers an uncertain Camunda start (for reconciliation).
ALTER TABLE vacation_requests DROP CONSTRAINT vacation_status_check;
ALTER TABLE vacation_requests ADD CONSTRAINT vacation_status_check CHECK (status IN (
    'pending_balance', 'pending_manager', 'auto_approved', 'approved',
    'rejected', 'needs_hr_review', 'start_unconfirmed'
));
