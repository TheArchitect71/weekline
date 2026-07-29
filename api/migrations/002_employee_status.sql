ALTER TABLE users
    ADD COLUMN IF NOT EXISTS employee_status TEXT NOT NULL DEFAULT 'active';

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS schedule_eligible BOOLEAN NOT NULL DEFAULT TRUE;

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS deactivated_at TIMESTAMPTZ;

UPDATE users
SET employee_status = 'deactivated',
    schedule_eligible = FALSE,
    deactivated_at = COALESCE(deactivated_at, updated_at, now())
WHERE role = 'worker'
  AND is_active = FALSE;

UPDATE users
SET schedule_eligible = FALSE
WHERE role = 'worker'
  AND employee_status <> 'active';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'users_employee_status_check'
    ) THEN
        ALTER TABLE users
            ADD CONSTRAINT users_employee_status_check
            CHECK (employee_status IN ('active', 'inactive', 'deactivated'));
    END IF;
END $$;
