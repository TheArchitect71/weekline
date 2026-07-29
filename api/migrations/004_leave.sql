CREATE TABLE IF NOT EXISTS leave_types (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    paid BOOLEAN NOT NULL DEFAULT TRUE,
    tracks_balance BOOLEAN NOT NULL DEFAULT TRUE,
    accrual_hours_per_month NUMERIC(7,2) NOT NULL DEFAULT 0 CHECK (accrual_hours_per_month >= 0),
    default_balance_hours NUMERIC(7,2) NOT NULL DEFAULT 0 CHECK (default_balance_hours >= 0),
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO leave_types (code, name, paid, tracks_balance, accrual_hours_per_month, default_balance_hours)
VALUES
    ('vacation', 'Vacation', TRUE, TRUE, 6.67, 80),
    ('sick', 'Sick leave', TRUE, TRUE, 4.00, 40),
    ('unpaid', 'Unpaid time off', FALSE, FALSE, 0, 0)
ON CONFLICT (code) DO NOTHING;

CREATE TABLE IF NOT EXISTS leave_balances (
    employee_id UUID NOT NULL REFERENCES users(id),
    leave_type_id UUID NOT NULL REFERENCES leave_types(id),
    balance_hours NUMERIC(7,2) NOT NULL DEFAULT 0 CHECK (balance_hours >= 0),
    used_hours NUMERIC(7,2) NOT NULL DEFAULT 0 CHECK (used_hours >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (employee_id, leave_type_id),
    CHECK (used_hours <= balance_hours)
);

CREATE INDEX IF NOT EXISTS leave_balances_type_idx ON leave_balances (leave_type_id);

CREATE TABLE IF NOT EXISTS leave_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    employee_id UUID NOT NULL REFERENCES users(id),
    leave_type_id UUID NOT NULL REFERENCES leave_types(id),
    starts_on DATE NOT NULL,
    ends_on DATE NOT NULL,
    hours_per_day NUMERIC(5,2) NOT NULL DEFAULT 8 CHECK (hours_per_day > 0 AND hours_per_day <= 24),
    requested_hours NUMERIC(7,2) NOT NULL CHECK (requested_hours > 0),
    reason VARCHAR(240) NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'approved', 'rejected', 'cancelled')),
    approved_by_user_id UUID REFERENCES users(id),
    approved_at TIMESTAMPTZ,
    resolved_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (ends_on >= starts_on)
);

CREATE INDEX IF NOT EXISTS leave_requests_employee_idx
    ON leave_requests (employee_id, created_at DESC);
CREATE INDEX IF NOT EXISTS leave_requests_type_idx ON leave_requests (leave_type_id);
CREATE INDEX IF NOT EXISTS leave_requests_approver_idx
    ON leave_requests (approved_by_user_id) WHERE approved_by_user_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS leave_requests_active_dates_idx
    ON leave_requests (employee_id, starts_on, ends_on)
    WHERE status IN ('pending', 'approved');
CREATE INDEX IF NOT EXISTS leave_requests_pending_idx
    ON leave_requests (created_at DESC) WHERE status = 'pending';

CREATE TABLE IF NOT EXISTS public_holidays (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    holiday_date DATE NOT NULL UNIQUE,
    name TEXT NOT NULL,
    paid BOOLEAN NOT NULL DEFAULT TRUE,
    created_by_user_id UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS public_holidays_creator_idx
    ON public_holidays (created_by_user_id) WHERE created_by_user_id IS NOT NULL;
