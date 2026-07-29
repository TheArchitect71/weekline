CREATE TABLE IF NOT EXISTS shift_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    request_type TEXT NOT NULL CHECK (request_type IN ('swap', 'open_claim', 'shift_change')),
    requested_by_employee_id UUID NOT NULL REFERENCES users(id),
    source_shift_id UUID NOT NULL,
    source_employee_id UUID NOT NULL REFERENCES users(id),
    source_date DATE NOT NULL,
    source_starts_at TIME NOT NULL,
    source_ends_at TIME NOT NULL,
    source_note VARCHAR(120) NOT NULL DEFAULT '',
    target_shift_id UUID,
    target_employee_id UUID REFERENCES users(id),
    target_date DATE,
    target_starts_at TIME,
    target_ends_at TIME,
    target_note VARCHAR(120) NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'approved', 'rejected', 'cancelled', 'expired')),
    expires_at TIMESTAMPTZ NOT NULL,
    resolved_at TIMESTAMPTZ,
    approved_by_user_id UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (request_type <> 'swap' OR (
        target_shift_id IS NOT NULL AND target_employee_id IS NOT NULL
        AND target_date IS NOT NULL AND target_starts_at IS NOT NULL AND target_ends_at IS NOT NULL
    ))
);

CREATE UNIQUE INDEX IF NOT EXISTS shift_requests_pending_source_idx
    ON shift_requests (source_shift_id)
    WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS shift_requests_requester_idx
    ON shift_requests (requested_by_employee_id, created_at DESC);

CREATE INDEX IF NOT EXISTS shift_requests_status_idx
    ON shift_requests (status, created_at DESC);
