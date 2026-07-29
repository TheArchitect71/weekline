CREATE TABLE IF NOT EXISTS manager_host_leases (
    instance_id UUID PRIMARY KEY,
    machine_name VARCHAR(120) NOT NULL,
    app_version VARCHAR(40) NOT NULL DEFAULT '',
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS manager_host_leases_expiry_idx
    ON manager_host_leases (expires_at);
