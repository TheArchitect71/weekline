CREATE TABLE IF NOT EXISTS open_shifts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    schedule_id UUID NOT NULL REFERENCES schedules(id) ON DELETE CASCADE,
    shift_date DATE NOT NULL,
    starts_at TIME NOT NULL,
    ends_at TIME NOT NULL,
    notes VARCHAR(120) NOT NULL DEFAULT '',
    required_headcount INTEGER NOT NULL DEFAULT 1 CHECK (required_headcount > 0 AND required_headcount <= 100),
    status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'closed')),
    change_version INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (ends_at > starts_at)
);

CREATE INDEX IF NOT EXISTS open_shifts_schedule_date_idx
    ON open_shifts (schedule_id, shift_date, starts_at);
CREATE INDEX IF NOT EXISTS open_shifts_active_idx
    ON open_shifts (schedule_id, shift_date) WHERE status = 'open';

ALTER TABLE shifts ADD COLUMN IF NOT EXISTS open_shift_id UUID REFERENCES open_shifts(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS shifts_open_shift_idx ON shifts (open_shift_id) WHERE open_shift_id IS NOT NULL;

ALTER TABLE published_shifts ADD COLUMN IF NOT EXISTS open_shift_id UUID;
CREATE INDEX IF NOT EXISTS published_shifts_open_shift_idx
    ON published_shifts (open_shift_id) WHERE open_shift_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS published_open_shifts (
    schedule_id UUID NOT NULL REFERENCES schedules(id) ON DELETE CASCADE,
    source_open_shift_id UUID NOT NULL,
    shift_date DATE NOT NULL,
    starts_at TIME NOT NULL,
    ends_at TIME NOT NULL,
    notes VARCHAR(120) NOT NULL DEFAULT '',
    required_headcount INTEGER NOT NULL,
    status TEXT NOT NULL,
    change_version INTEGER NOT NULL,
    PRIMARY KEY (schedule_id, source_open_shift_id)
);

CREATE INDEX IF NOT EXISTS published_open_shifts_date_idx
    ON published_open_shifts (schedule_id, shift_date, starts_at);

ALTER TABLE shift_requests ADD COLUMN IF NOT EXISTS open_shift_id UUID;
CREATE INDEX IF NOT EXISTS shift_requests_open_shift_idx
    ON shift_requests (open_shift_id) WHERE open_shift_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS shift_requests_pending_open_claim_idx
    ON shift_requests (requested_by_employee_id, open_shift_id)
    WHERE status = 'pending' AND request_type = 'open_claim';
