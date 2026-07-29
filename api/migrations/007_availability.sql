CREATE TABLE IF NOT EXISTS availability_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    employee_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    availability_kind TEXT NOT NULL CHECK (availability_kind IN ('unavailable', 'preferred')),
    day_of_week SMALLINT NOT NULL CHECK (day_of_week BETWEEN 1 AND 7),
    starts_at TIME NOT NULL,
    ends_at TIME NOT NULL,
    effective_from DATE,
    effective_until DATE,
    notes VARCHAR(120) NOT NULL DEFAULT '',
    created_by_user_id UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (ends_at > starts_at),
    CHECK (effective_until IS NULL OR effective_from IS NULL OR effective_until >= effective_from)
);

CREATE INDEX IF NOT EXISTS availability_rules_employee_day_idx
    ON availability_rules (employee_id, day_of_week, starts_at);
CREATE INDEX IF NOT EXISTS availability_rules_effective_idx
    ON availability_rules (effective_from, effective_until);
CREATE INDEX IF NOT EXISTS availability_rules_creator_idx
    ON availability_rules (created_by_user_id);
