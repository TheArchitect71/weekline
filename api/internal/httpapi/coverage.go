package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"weekline/api/internal/store"
)

func (h *Handler) createOpenShift(w http.ResponseWriter, r *http.Request) {
	if !requireJSON(w, r) {
		return
	}
	week := r.PathValue("week")
	if err := validateWeek(week); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_week", err.Error())
		return
	}
	var input store.OpenShiftInput
	if err := decodeJSON(r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := validateOpenShift(input, week); err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_open_shift", err.Error())
		return
	}
	openShift, err := h.store.CreateOpenShift(r.Context(), week, currentUser(r).ID, input)
	if h.handleStoreError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, openShift)
}

func (h *Handler) updateOpenShift(w http.ResponseWriter, r *http.Request) {
	if !requireJSON(w, r) {
		return
	}
	var input store.OpenShiftInput
	if err := decodeJSON(r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := validateOpenShift(input, ""); err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_open_shift", err.Error())
		return
	}
	openShift, err := h.store.UpdateOpenShift(r.Context(), r.PathValue("openShiftID"), currentUser(r).ID, input)
	if h.handleStoreError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, openShift)
}

func (h *Handler) deleteOpenShift(w http.ResponseWriter, r *http.Request) {
	if err := h.store.DeleteOpenShift(r.Context(), r.PathValue("openShiftID"), currentUser(r).ID); h.handleStoreError(w, err) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) openShiftSuggestions(w http.ResponseWriter, r *http.Request) {
	suggestions, err := h.store.OpenShiftSuggestions(r.Context(), r.PathValue("openShiftID"))
	if h.handleStoreError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, suggestions)
}

func (h *Handler) assignOpenShift(w http.ResponseWriter, r *http.Request) {
	if !requireJSON(w, r) {
		return
	}
	var input struct {
		EmployeeID string `json:"employeeId"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if strings.TrimSpace(input.EmployeeID) == "" {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_assignment", "Employee is required.")
		return
	}
	shift, err := h.store.AssignOpenShift(r.Context(), r.PathValue("openShiftID"), input.EmployeeID, currentUser(r).ID)
	if h.handleStoreError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, shift)
}

func (h *Handler) claimOpenShift(w http.ResponseWriter, r *http.Request) {
	request, err := h.store.CreateOpenClaim(r.Context(), r.PathValue("openShiftID"), currentUser(r))
	if h.handleStoreError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, request)
}

func validateOpenShift(input store.OpenShiftInput, week string) error {
	if _, err := time.Parse("2006-01-02", input.Date); err != nil {
		return errors.New("Date must use YYYY-MM-DD format.")
	}
	if week != "" && !dateInWeek(input.Date, week) {
		return errors.New("Open shift date must fall within the selected week.")
	}
	start, startErr := time.Parse("15:04", input.Start)
	end, endErr := time.Parse("15:04", input.End)
	if startErr != nil || endErr != nil {
		return errors.New("Times must use 24-hour HH:MM format.")
	}
	if !end.After(start) {
		return errors.New("End time must be later than start time.")
	}
	if input.RequiredHeadcount < 1 || input.RequiredHeadcount > 100 {
		return errors.New("Required headcount must be between 1 and 100.")
	}
	if utf8.RuneCountInString(input.Note) > 120 {
		return errors.New("Notes cannot exceed 120 characters.")
	}
	return nil
}

func dateInWeek(value, weekValue string) bool {
	week, weekErr := time.Parse("2006-01-02", weekValue)
	date, dateErr := time.Parse("2006-01-02", value)
	return weekErr == nil && dateErr == nil && !date.Before(week) && !date.After(week.AddDate(0, 0, 6))
}
