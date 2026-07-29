package store

import (
	"context"
	"time"
)

const defaultHostLeaseTTL = 45 * time.Second

func (p *Postgres) RenewHostLease(ctx context.Context, input HostLeaseInput, ttl time.Duration) (HostStatus, error) {
	if ttl <= 0 {
		ttl = defaultHostLeaseTTL
	}
	expiresAt := time.Now().UTC().Add(ttl)
	if _, err := p.db.Exec(ctx, `
		INSERT INTO manager_host_leases (instance_id, machine_name, app_version, last_seen_at, expires_at)
		VALUES ($1::uuid, $2, $3, now(), $4)
		ON CONFLICT (instance_id) DO UPDATE
		SET machine_name = EXCLUDED.machine_name,
		    app_version = EXCLUDED.app_version,
		    last_seen_at = now(),
		    expires_at = EXCLUDED.expires_at`,
		input.InstanceID, input.MachineName, input.AppVersion, expiresAt,
	); err != nil {
		return HostStatus{}, err
	}
	return p.HostStatus(ctx)
}

func (p *Postgres) ReleaseHostLease(ctx context.Context, instanceID string) (HostStatus, error) {
	if _, err := p.db.Exec(ctx, `DELETE FROM manager_host_leases WHERE instance_id = $1::uuid`, instanceID); err != nil {
		return HostStatus{}, err
	}
	return p.HostStatus(ctx)
}

func (p *Postgres) HostStatus(ctx context.Context) (HostStatus, error) {
	if _, err := p.db.Exec(ctx, `DELETE FROM manager_host_leases WHERE expires_at <= now()`); err != nil {
		return HostStatus{}, err
	}
	rows, err := p.db.Query(ctx, `
		SELECT instance_id::text, machine_name, app_version,
		       to_char(last_seen_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       to_char(expires_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM manager_host_leases
		WHERE expires_at > now()
		ORDER BY machine_name, instance_id`)
	if err != nil {
		return HostStatus{}, err
	}
	defer rows.Close()

	status := HostStatus{Instances: []HostLease{}}
	for rows.Next() {
		var lease HostLease
		if err := rows.Scan(&lease.InstanceID, &lease.MachineName, &lease.AppVersion, &lease.LastSeenAt, &lease.ExpiresAt); err != nil {
			return HostStatus{}, err
		}
		status.Instances = append(status.Instances, lease)
	}
	if err := rows.Err(); err != nil {
		return HostStatus{}, err
	}
	status.ActiveCount = len(status.Instances)
	status.Available = status.ActiveCount > 0
	return status, nil
}

func (p *Postgres) HasActiveHostLease(ctx context.Context) (bool, error) {
	var active bool
	err := p.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM manager_host_leases WHERE expires_at > now()
		)`).Scan(&active)
	return active, err
}
