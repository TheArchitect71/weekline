CREATE TABLE IF NOT EXISTS attendance_stations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    token_hash BYTEA NOT NULL UNIQUE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_by_user_id UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS attendance_stations_creator_idx
    ON attendance_stations (created_by_user_id);

CREATE TABLE IF NOT EXISTS attendance_corrections (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    employee_id UUID NOT NULL REFERENCES users(id),
    attendance_date DATE NOT NULL,
    proposed_clock_in TIME NOT NULL,
    proposed_clock_out TIME NOT NULL,
    reason VARCHAR(240) NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected', 'cancelled')),
    requested_by_user_id UUID NOT NULL REFERENCES users(id),
    resolved_by_user_id UUID REFERENCES users(id),
    resolved_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (proposed_clock_out > proposed_clock_in)
);

CREATE INDEX IF NOT EXISTS attendance_corrections_employee_date_idx
    ON attendance_corrections (employee_id, attendance_date DESC);
CREATE INDEX IF NOT EXISTS attendance_corrections_pending_idx
    ON attendance_corrections (created_at) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS attendance_corrections_requester_idx
    ON attendance_corrections (requested_by_user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS attendance_corrections_resolver_idx
    ON attendance_corrections (resolved_by_user_id) WHERE resolved_by_user_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS attendance_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    station_id UUID REFERENCES attendance_stations(id),
    employee_id UUID NOT NULL REFERENCES users(id),
    event_type TEXT NOT NULL CHECK (event_type IN ('in', 'out')),
    event_source TEXT NOT NULL CHECK (event_source IN ('station', 'correction')),
    occurred_at TIMESTAMPTZ NOT NULL,
    client_event_id UUID NOT NULL UNIQUE,
    correction_id UUID REFERENCES attendance_corrections(id),
    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((event_source = 'station' AND station_id IS NOT NULL AND correction_id IS NULL)
        OR (event_source = 'correction' AND station_id IS NULL AND correction_id IS NOT NULL))
);

CREATE INDEX IF NOT EXISTS attendance_events_employee_time_idx
    ON attendance_events (employee_id, occurred_at DESC);
CREATE INDEX IF NOT EXISTS attendance_events_station_time_idx
    ON attendance_events (station_id, occurred_at DESC) WHERE station_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS attendance_events_correction_idx
    ON attendance_events (correction_id) WHERE correction_id IS NOT NULL;
