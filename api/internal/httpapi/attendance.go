package httpapi

import (
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"weekline/api/internal/store"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)

func (h *Handler) attendanceStations(w http.ResponseWriter, r *http.Request) {
	stations, err := h.store.ListAttendanceStations(r.Context())
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "server_error", "Unable to load attendance stations.")
		return
	}
	writeJSON(w, http.StatusOK, stations)
}

func (h *Handler) createAttendanceStation(w http.ResponseWriter, r *http.Request) {
	if !requireJSON(w, r) {
		return
	}
	var input struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := validateStationName(input.Name); err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_station", err.Error())
		return
	}
	station, err := h.store.CreateAttendanceStation(r.Context(), currentUser(r).ID, input.Name)
	if h.handleStoreError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, station)
}

func (h *Handler) updateAttendanceStation(w http.ResponseWriter, r *http.Request) {
	if !requireJSON(w, r) {
		return
	}
	var input struct {
		Name   string `json:"name"`
		Active bool   `json:"active"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := validateStationName(input.Name); err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_station", err.Error())
		return
	}
	station, err := h.store.UpdateAttendanceStation(r.Context(), r.PathValue("stationID"), currentUser(r).ID, input.Name, input.Active)
	if h.handleStoreError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, station)
}

func (h *Handler) rotateAttendanceStation(w http.ResponseWriter, r *http.Request) {
	station, err := h.store.RotateAttendanceStationToken(r.Context(), r.PathValue("stationID"), currentUser(r).ID)
	if h.handleStoreError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, station)
}

func (h *Handler) kioskSession(w http.ResponseWriter, r *http.Request) {
	session, err := h.store.KioskSession(r.Context(), r.PathValue("token"))
	if h.handleStoreError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, session)
}

func (h *Handler) createPunch(w http.ResponseWriter, r *http.Request) {
	if !requireJSON(w, r) {
		return
	}
	var input store.PunchInput
	if err := decodeJSON(r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := validatePunch(input); err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_punch", err.Error())
		return
	}
	event, err := h.store.CreatePunch(r.Context(), r.PathValue("token"), input)
	if h.handleStoreError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, event)
}

func (h *Handler) attendance(w http.ResponseWriter, r *http.Request) {
	from := strings.TrimSpace(r.URL.Query().Get("from"))
	to := strings.TrimSpace(r.URL.Query().Get("to"))
	if err := validateDateRange(from, to, 62); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_date_range", err.Error())
		return
	}
	records, err := h.store.ListAttendance(r.Context(), from, to, currentUser(r), strings.TrimSpace(r.URL.Query().Get("employeeId")))
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "server_error", "Unable to load attendance.")
		return
	}
	writeJSON(w, http.StatusOK, records)
}

func (h *Handler) attendanceCorrections(w http.ResponseWriter, r *http.Request) {
	corrections, err := h.store.ListAttendanceCorrections(r.Context(), currentUser(r))
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "server_error", "Unable to load attendance corrections.")
		return
	}
	writeJSON(w, http.StatusOK, corrections)
}

func (h *Handler) createAttendanceCorrection(w http.ResponseWriter, r *http.Request) {
	if !requireJSON(w, r) {
		return
	}
	var input store.AttendanceCorrectionInput
	if err := decodeJSON(r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := validateAttendanceCorrection(input, currentUser(r).Role == "manager"); err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_correction", err.Error())
		return
	}
	correction, err := h.store.CreateAttendanceCorrection(r.Context(), currentUser(r), input)
	if h.handleStoreError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, correction)
}

func (h *Handler) approveAttendanceCorrection(w http.ResponseWriter, r *http.Request) {
	h.resolveAttendanceCorrection(w, r, "approved")
}

func (h *Handler) rejectAttendanceCorrection(w http.ResponseWriter, r *http.Request) {
	h.resolveAttendanceCorrection(w, r, "rejected")
}

func (h *Handler) resolveAttendanceCorrection(w http.ResponseWriter, r *http.Request, decision string) {
	correction, err := h.store.ResolveAttendanceCorrection(r.Context(), r.PathValue("correctionID"), currentUser(r).ID, decision)
	if h.handleStoreError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, correction)
}

func (h *Handler) cancelAttendanceCorrection(w http.ResponseWriter, r *http.Request) {
	correction, err := h.store.CancelAttendanceCorrection(r.Context(), r.PathValue("correctionID"), currentUser(r).ID)
	if h.handleStoreError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, correction)
}

func validateStationName(name string) error {
	length := utf8.RuneCountInString(strings.TrimSpace(name))
	if length < 2 || length > 80 {
		return errors.New("Station name must be between 2 and 80 characters.")
	}
	return nil
}

func validatePunch(input store.PunchInput) error {
	if !uuidPattern.MatchString(input.EmployeeID) || !uuidPattern.MatchString(input.ClientEventID) {
		return errors.New("Employee and client event IDs must be valid UUIDs.")
	}
	if input.EventType != "in" && input.EventType != "out" {
		return errors.New("Punch type must be in or out.")
	}
	if input.OccurredAt != "" {
		occurredAt, err := time.Parse(time.RFC3339, input.OccurredAt)
		if err != nil {
			return errors.New("Punch timestamp must use RFC3339 format.")
		}
		now := time.Now()
		if occurredAt.Before(now.Add(-24*time.Hour)) || occurredAt.After(now.Add(5*time.Minute)) {
			return errors.New("Offline punches must be uploaded within 24 hours.")
		}
	}
	return nil
}

func validateAttendanceCorrection(input store.AttendanceCorrectionInput, employeeRequired bool) error {
	if employeeRequired && !uuidPattern.MatchString(input.EmployeeID) {
		return errors.New("Employee is required.")
	}
	if _, err := time.Parse("2006-01-02", input.Date); err != nil {
		return errors.New("Date must use YYYY-MM-DD format.")
	}
	clockIn, inErr := time.Parse("15:04", input.ClockIn)
	clockOut, outErr := time.Parse("15:04", input.ClockOut)
	if inErr != nil || outErr != nil || !clockOut.After(clockIn) {
		return errors.New("Clock out must be later than clock in.")
	}
	length := utf8.RuneCountInString(strings.TrimSpace(input.Reason))
	if length < 3 || length > 240 {
		return errors.New("Reason must be between 3 and 240 characters.")
	}
	return nil
}

func validateDateRange(from, to string, maxDays int) error {
	start, startErr := time.Parse("2006-01-02", from)
	end, endErr := time.Parse("2006-01-02", to)
	if startErr != nil || endErr != nil {
		return errors.New("Dates must use YYYY-MM-DD format.")
	}
	if end.Before(start) {
		return errors.New("End date cannot be before start date.")
	}
	if end.Sub(start) > time.Duration(maxDays)*24*time.Hour {
		return errors.New("Date range is too large.")
	}
	return nil
}
