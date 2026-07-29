CREATE TABLE IF NOT EXISTS scheduling_policies (
    id BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (id),
    overtime_warning_hours NUMERIC(5,2) NOT NULL DEFAULT 40 CHECK (overtime_warning_hours > 0),
    maximum_weekly_hours NUMERIC(5,2) NOT NULL DEFAULT 60 CHECK (maximum_weekly_hours > 0),
    minimum_rest_hours NUMERIC(5,2) NOT NULL DEFAULT 8 CHECK (minimum_rest_hours >= 0),
    minimum_shift_hours NUMERIC(5,2) NOT NULL DEFAULT 2 CHECK (minimum_shift_hours > 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (maximum_weekly_hours >= overtime_warning_hours)
);

INSERT INTO scheduling_policies (id) VALUES (TRUE) ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS conflict_overrides (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    schedule_id UUID NOT NULL REFERENCES schedules(id) ON DELETE CASCADE,
    shift_id UUID REFERENCES shifts(id) ON DELETE SET NULL,
    employee_id UUID NOT NULL REFERENCES users(id),
    conflict_code TEXT NOT NULL,
    severity TEXT NOT NULL CHECK (severity IN ('info', 'warning', 'blocking')),
    message TEXT NOT NULL,
    can_override BOOLEAN NOT NULL,
    resolution TEXT NOT NULL CHECK (resolution IN ('accepted', 'overridden')),
    actor_id UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS conflict_overrides_schedule_idx
    ON conflict_overrides (schedule_id, created_at DESC);
CREATE INDEX IF NOT EXISTS conflict_overrides_shift_idx
    ON conflict_overrides (shift_id) WHERE shift_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS conflict_overrides_employee_idx
    ON conflict_overrides (employee_id, created_at DESC);
CREATE INDEX IF NOT EXISTS conflict_overrides_actor_idx
    ON conflict_overrides (actor_id, created_at DESC);
