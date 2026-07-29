package store

import (
	"context"
	"fmt"
	"net/mail"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

func (p *Postgres) BootstrapManager(ctx context.Context, input BootstrapManagerInput) (User, bool, error) {
	email := strings.ToLower(strings.TrimSpace(input.Email))
	displayName := strings.TrimSpace(input.DisplayName)
	address, addressErr := mail.ParseAddress(email)
	if addressErr != nil || !strings.EqualFold(address.Address, email) || len(email) > 254 {
		return User{}, false, fmt.Errorf("%w: manager email is invalid", ErrInvalid)
	}
	if displayName == "" || utf8.RuneCountInString(displayName) > 120 {
		return User{}, false, fmt.Errorf("%w: manager name is required and must be 120 characters or fewer", ErrInvalid)
	}
	if len(input.Password) < 12 || len(input.Password) > 72 {
		return User{}, false, fmt.Errorf("%w: manager password must contain 12 to 72 bytes", ErrInvalid)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, false, err
	}
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return User{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext('weekline-bootstrap-manager'))"); err != nil {
		return User{}, false, err
	}
	var managerExists bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM users WHERE role = 'manager')").Scan(&managerExists); err != nil {
		return User{}, false, err
	}
	if managerExists {
		return User{}, false, tx.Commit(ctx)
	}

	var manager User
	err = tx.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, display_name, role, schedule_eligible)
		VALUES ($1, $2, $3, 'manager', false)
		RETURNING id::text, email, display_name, role`,
		email, string(hash), displayName,
	).Scan(&manager.ID, &manager.Email, &manager.DisplayName, &manager.Role)
	if err != nil {
		if isUniqueViolation(err) {
			return User{}, false, fmt.Errorf("%w: email is already in use", ErrConflict)
		}
		return User{}, false, err
	}
	if err = insertAudit(ctx, tx, manager.ID, "user", manager.ID, "bootstrapped", nil, map[string]any{
		"email": manager.Email, "displayName": manager.DisplayName, "role": manager.Role,
	}); err != nil {
		return User{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return User{}, false, err
	}
	return manager, true, nil
}
