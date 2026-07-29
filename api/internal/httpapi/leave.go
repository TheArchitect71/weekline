package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"weekline/api/internal/store"
)

func (h *Handler) leaveTypes(w http.ResponseWriter, r *http.Request) {
	types, err := h.store.ListLeaveTypes(r.Context())
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "server_error", "Unable to load leave types.")
		return
	}
	writeJSON(w, http.StatusOK, types)
}

func (h *Handler) leaveBalances(w http.ResponseWriter, r *http.Request) {
	balances, err := h.store.ListLeaveBalances(r.Context(), currentUser(r))
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "server_error", "Unable to load leave balances.")
		return
	}
	writeJSON(w, http.StatusOK, balances)
}

func (h *Handler) setLeaveBalance(w http.ResponseWriter, r *http.Request) {
	if !requireJSON(w, r) {
		return
	}
	var input struct {
		BalanceHours float64 `json:"balanceHours"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if input.BalanceHours < 0 || input.BalanceHours > 10000 {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_balance", "Balance must be between 0 and 10,000 hours.")
		return
	}
	balance, err := h.store.SetLeaveBalance(r.Context(), r.PathValue("employeeID"), r.PathValue("leaveTypeID"), currentUser(r).ID, input.BalanceHours)
	if h.handleStoreError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, balance)
}

func (h *Handler) leaveRequests(w http.ResponseWriter, r *http.Request) {
	requests, err := h.store.ListLeaveRequests(r.Context(), currentUser(r))
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "server_error", "Unable to load leave requests.")
		return
	}
	writeJSON(w, http.StatusOK, requests)
}

func (h *Handler) createLeaveRequest(w http.ResponseWriter, r *http.Request) {
	if !requireJSON(w, r) {
		return
	}
	var input store.LeaveRequestInput
	if err := decodeJSON(r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	user := currentUser(r)
	employeeID := user.ID
	if user.Role == "manager" {
		employeeID = strings.TrimSpace(input.EmployeeID)
	}
	if employeeID == "" {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_leave_request", "Employee is required.")
		return
	}
	if err := validateLeaveRequest(input); err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_leave_request", err.Error())
		return
	}
	request, err := h.store.CreateLeaveRequest(r.Context(), employeeID, user.ID, input)
	if h.handleStoreError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, request)
}

func (h *Handler) approveLeaveRequest(w http.ResponseWriter, r *http.Request) {
	h.resolveLeaveRequest(w, r, "approved")
}

func (h *Handler) rejectLeaveRequest(w http.ResponseWriter, r *http.Request) {
	h.resolveLeaveRequest(w, r, "rejected")
}

func (h *Handler) resolveLeaveRequest(w http.ResponseWriter, r *http.Request, decision string) {
	request, err := h.store.ResolveLeaveRequest(r.Context(), r.PathValue("requestID"), currentUser(r).ID, decision)
	if h.handleStoreError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, request)
}

func (h *Handler) cancelLeaveRequest(w http.ResponseWriter, r *http.Request) {
	request, err := h.store.CancelLeaveRequest(r.Context(), r.PathValue("requestID"), currentUser(r))
	if h.handleStoreError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, request)
}

func (h *Handler) publicHolidays(w http.ResponseWriter, r *http.Request) {
	startsOn := strings.TrimSpace(r.URL.Query().Get("startsOn"))
	endsOn := strings.TrimSpace(r.URL.Query().Get("endsOn"))
	if (startsOn == "") != (endsOn == "") {
		writeProblem(w, http.StatusBadRequest, "invalid_range", "Both startsOn and endsOn are required for a date range.")
		return
	}
	if startsOn != "" {
		if _, err := parseDateRange(startsOn, endsOn); err != nil {
			writeProblem(w, http.StatusBadRequest, "invalid_range", err.Error())
			return
		}
	}
	holidays, err := h.store.ListPublicHolidays(r.Context(), startsOn, endsOn)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "server_error", "Unable to load public holidays.")
		return
	}
	writeJSON(w, http.StatusOK, holidays)
}

func (h *Handler) createPublicHoliday(w http.ResponseWriter, r *http.Request) {
	if !requireJSON(w, r) {
		return
	}
	var input store.PublicHolidayInput
	if err := decodeJSON(r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if _, err := time.Parse("2006-01-02", input.Date); err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_holiday", "Holiday date must use YYYY-MM-DD format.")
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || utf8.RuneCountInString(input.Name) > 80 {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_holiday", "Holiday name is required and cannot exceed 80 characters.")
		return
	}
	holiday, err := h.store.CreatePublicHoliday(r.Context(), currentUser(r).ID, input)
	if h.handleStoreError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, holiday)
}

func (h *Handler) deletePublicHoliday(w http.ResponseWriter, r *http.Request) {
	if err := h.store.DeletePublicHoliday(r.Context(), r.PathValue("holidayID"), currentUser(r).ID); h.handleStoreError(w, err) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func validateLeaveRequest(input store.LeaveRequestInput) error {
	if strings.TrimSpace(input.LeaveTypeID) == "" {
		return errors.New("Leave type is required.")
	}
	if _, err := parseDateRange(input.StartsOn, input.EndsOn); err != nil {
		return err
	}
	if input.HoursPerDay <= 0 || input.HoursPerDay > 24 {
		return errors.New("Hours per day must be greater than 0 and no more than 24.")
	}
	if utf8.RuneCountInString(input.Reason) > 240 {
		return errors.New("Reason cannot exceed 240 characters.")
	}
	return nil
}

func parseDateRange(startsOn, endsOn string) (int, error) {
	start, startErr := time.Parse("2006-01-02", startsOn)
	end, endErr := time.Parse("2006-01-02", endsOn)
	if startErr != nil || endErr != nil {
		return 0, errors.New("Dates must use YYYY-MM-DD format.")
	}
	if end.Before(start) {
		return 0, errors.New("End date must be on or after start date.")
	}
	days := int(end.Sub(start).Hours()/24) + 1
	if days > 366 {
		return 0, errors.New("Leave requests cannot exceed 366 days.")
	}
	return days, nil
}
