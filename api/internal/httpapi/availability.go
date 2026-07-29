package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"weekline/api/internal/store"
)

func (h *Handler) availability(w http.ResponseWriter, r *http.Request) {
	rules, err := h.store.ListAvailability(r.Context(), currentUser(r))
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "server_error", "Unable to load availability.")
		return
	}
	writeJSON(w, http.StatusOK, rules)
}

func (h *Handler) createAvailability(w http.ResponseWriter, r *http.Request) {
	if !requireJSON(w, r) {
		return
	}
	var input store.AvailabilityInput
	if err := decodeJSON(r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := validateAvailability(input, currentUser(r).Role == "manager"); err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_availability", err.Error())
		return
	}
	rule, err := h.store.CreateAvailability(r.Context(), currentUser(r), input)
	if h.handleStoreError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, rule)
}

func (h *Handler) updateAvailability(w http.ResponseWriter, r *http.Request) {
	if !requireJSON(w, r) {
		return
	}
	var input store.AvailabilityInput
	if err := decodeJSON(r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := validateAvailability(input, currentUser(r).Role == "manager"); err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_availability", err.Error())
		return
	}
	rule, err := h.store.UpdateAvailability(r.Context(), r.PathValue("ruleID"), currentUser(r), input)
	if h.handleStoreError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, rule)
}

func (h *Handler) deleteAvailability(w http.ResponseWriter, r *http.Request) {
	if err := h.store.DeleteAvailability(r.Context(), r.PathValue("ruleID"), currentUser(r)); h.handleStoreError(w, err) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func validateAvailability(input store.AvailabilityInput, employeeRequired bool) error {
	if employeeRequired && strings.TrimSpace(input.EmployeeID) == "" {
		return errors.New("Employee is required.")
	}
	if input.Kind != "unavailable" && input.Kind != "preferred" {
		return errors.New("Availability type must be unavailable or preferred.")
	}
	if input.DayOfWeek < 1 || input.DayOfWeek > 7 {
		return errors.New("Day of week must be between Monday and Sunday.")
	}
	start, startErr := time.Parse("15:04", input.Start)
	end, endErr := time.Parse("15:04", input.End)
	if startErr != nil || endErr != nil || !end.After(start) {
		return errors.New("End time must be later than start time.")
	}
	var from, until time.Time
	var err error
	if input.EffectiveFrom != "" {
		from, err = time.Parse("2006-01-02", input.EffectiveFrom)
		if err != nil {
			return errors.New("Effective start must use YYYY-MM-DD format.")
		}
	}
	if input.EffectiveUntil != "" {
		until, err = time.Parse("2006-01-02", input.EffectiveUntil)
		if err != nil {
			return errors.New("Effective end must use YYYY-MM-DD format.")
		}
	}
	if !from.IsZero() && !until.IsZero() && until.Before(from) {
		return errors.New("Effective end cannot be before effective start.")
	}
	if utf8.RuneCountInString(input.Note) > 120 {
		return errors.New("Notes cannot exceed 120 characters.")
	}
	return nil
}
