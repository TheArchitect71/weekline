package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"

	"weekline/api/internal/store"
)

const sessionCookie = "weekline_session"

type Config struct {
	CookieSecure     bool
	SessionTTL       time.Duration
	HostControlToken string
	HostLeaseTTL     time.Duration
	RequireHostLease bool
	EnableAttendance bool
}

type Handler struct {
	store  *store.Postgres
	config Config
}

type contextKey string

const userContextKey contextKey = "user"

func NewHandler(data *store.Postgres, config Config) http.Handler {
	if config.SessionTTL == 0 {
		config.SessionTTL = 8 * time.Hour
	}
	h := &Handler{store: data, config: config}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", h.health)
	mux.HandleFunc("GET /api/host/availability", h.hostAvailability)
	mux.Handle("PUT /api/host/leases/{instanceID}", h.withHostControl(http.HandlerFunc(h.renewHostLease)))
	mux.Handle("DELETE /api/host/leases/{instanceID}", h.withHostControl(http.HandlerFunc(h.releaseHostLease)))
	mux.HandleFunc("POST /api/v1/auth/login", h.login)
	mux.HandleFunc("POST /api/v1/auth/logout", h.logout)
	if config.EnableAttendance {
		mux.HandleFunc("GET /api/v1/kiosk/{token}", h.kioskSession)
		mux.HandleFunc("POST /api/v1/kiosk/{token}/punch", h.createPunch)
	}
	mux.Handle("GET /api/v1/auth/me", h.withAuth(http.HandlerFunc(h.me)))
	mux.Handle("GET /api/v1/host/status", h.withAuth(h.managerOnly(http.HandlerFunc(h.hostStatus))))
	mux.Handle("GET /api/v1/schedules/{week}", h.withAuth(http.HandlerFunc(h.getSchedule)))
	mux.Handle("POST /api/v1/schedules/{week}/conflicts/evaluate", h.withAuth(h.managerOnly(http.HandlerFunc(h.evaluateShiftConflicts))))
	mux.Handle("POST /api/v1/schedules/{week}/shifts", h.withAuth(h.managerOnly(http.HandlerFunc(h.createShift))))
	mux.Handle("PUT /api/v1/shifts/{shiftID}", h.withAuth(h.managerOnly(http.HandlerFunc(h.updateShift))))
	mux.Handle("DELETE /api/v1/shifts/{shiftID}", h.withAuth(h.managerOnly(http.HandlerFunc(h.deleteShift))))
	mux.Handle("POST /api/v1/shifts/swap", h.withAuth(h.managerOnly(http.HandlerFunc(h.swapDraftShifts))))
	mux.Handle("GET /api/v1/shift-requests", h.withAuth(http.HandlerFunc(h.shiftRequests)))
	mux.Handle("GET /api/v1/shift-requests/swap-options", h.withAuth(h.workerOnly(http.HandlerFunc(h.swapOptions))))
	mux.Handle("POST /api/v1/shift-requests/swaps", h.withAuth(h.workerOnly(http.HandlerFunc(h.createSwapRequest))))
	mux.Handle("POST /api/v1/shift-requests/{requestID}/approve", h.withAuth(h.managerOnly(http.HandlerFunc(h.approveShiftRequest))))
	mux.Handle("POST /api/v1/shift-requests/{requestID}/reject", h.withAuth(h.managerOnly(http.HandlerFunc(h.rejectShiftRequest))))
	mux.Handle("POST /api/v1/shift-requests/{requestID}/cancel", h.withAuth(h.workerOnly(http.HandlerFunc(h.cancelShiftRequest))))
	mux.Handle("GET /api/v1/leave/types", h.withAuth(http.HandlerFunc(h.leaveTypes)))
	mux.Handle("GET /api/v1/leave/balances", h.withAuth(http.HandlerFunc(h.leaveBalances)))
	mux.Handle("PUT /api/v1/leave/balances/{employeeID}/{leaveTypeID}", h.withAuth(h.managerOnly(http.HandlerFunc(h.setLeaveBalance))))
	mux.Handle("GET /api/v1/leave/requests", h.withAuth(http.HandlerFunc(h.leaveRequests)))
	mux.Handle("POST /api/v1/leave/requests", h.withAuth(http.HandlerFunc(h.createLeaveRequest)))
	mux.Handle("POST /api/v1/leave/requests/{requestID}/approve", h.withAuth(h.managerOnly(http.HandlerFunc(h.approveLeaveRequest))))
	mux.Handle("POST /api/v1/leave/requests/{requestID}/reject", h.withAuth(h.managerOnly(http.HandlerFunc(h.rejectLeaveRequest))))
	mux.Handle("POST /api/v1/leave/requests/{requestID}/cancel", h.withAuth(http.HandlerFunc(h.cancelLeaveRequest)))
	mux.Handle("GET /api/v1/holidays", h.withAuth(http.HandlerFunc(h.publicHolidays)))
	mux.Handle("POST /api/v1/holidays", h.withAuth(h.managerOnly(http.HandlerFunc(h.createPublicHoliday))))
	mux.Handle("DELETE /api/v1/holidays/{holidayID}", h.withAuth(h.managerOnly(http.HandlerFunc(h.deletePublicHoliday))))
	mux.Handle("POST /api/v1/schedules/{week}/open-shifts", h.withAuth(h.managerOnly(http.HandlerFunc(h.createOpenShift))))
	mux.Handle("PUT /api/v1/open-shifts/{openShiftID}", h.withAuth(h.managerOnly(http.HandlerFunc(h.updateOpenShift))))
	mux.Handle("DELETE /api/v1/open-shifts/{openShiftID}", h.withAuth(h.managerOnly(http.HandlerFunc(h.deleteOpenShift))))
	mux.Handle("GET /api/v1/open-shifts/{openShiftID}/suggestions", h.withAuth(h.managerOnly(http.HandlerFunc(h.openShiftSuggestions))))
	mux.Handle("POST /api/v1/open-shifts/{openShiftID}/assign", h.withAuth(h.managerOnly(http.HandlerFunc(h.assignOpenShift))))
	mux.Handle("POST /api/v1/open-shifts/{openShiftID}/claim", h.withAuth(h.workerOnly(http.HandlerFunc(h.claimOpenShift))))
	mux.Handle("GET /api/v1/availability", h.withAuth(http.HandlerFunc(h.availability)))
	mux.Handle("POST /api/v1/availability", h.withAuth(http.HandlerFunc(h.createAvailability)))
	mux.Handle("PUT /api/v1/availability/{ruleID}", h.withAuth(http.HandlerFunc(h.updateAvailability)))
	mux.Handle("DELETE /api/v1/availability/{ruleID}", h.withAuth(http.HandlerFunc(h.deleteAvailability)))
	if config.EnableAttendance {
		mux.Handle("GET /api/v1/attendance", h.withAuth(http.HandlerFunc(h.attendance)))
		mux.Handle("GET /api/v1/attendance/stations", h.withAuth(h.managerOnly(http.HandlerFunc(h.attendanceStations))))
		mux.Handle("POST /api/v1/attendance/stations", h.withAuth(h.managerOnly(http.HandlerFunc(h.createAttendanceStation))))
		mux.Handle("PUT /api/v1/attendance/stations/{stationID}", h.withAuth(h.managerOnly(http.HandlerFunc(h.updateAttendanceStation))))
		mux.Handle("POST /api/v1/attendance/stations/{stationID}/rotate-token", h.withAuth(h.managerOnly(http.HandlerFunc(h.rotateAttendanceStation))))
		mux.Handle("GET /api/v1/attendance/corrections", h.withAuth(http.HandlerFunc(h.attendanceCorrections)))
		mux.Handle("POST /api/v1/attendance/corrections", h.withAuth(http.HandlerFunc(h.createAttendanceCorrection)))
		mux.Handle("POST /api/v1/attendance/corrections/{correctionID}/approve", h.withAuth(h.managerOnly(http.HandlerFunc(h.approveAttendanceCorrection))))
		mux.Handle("POST /api/v1/attendance/corrections/{correctionID}/reject", h.withAuth(h.managerOnly(http.HandlerFunc(h.rejectAttendanceCorrection))))
		mux.Handle("POST /api/v1/attendance/corrections/{correctionID}/cancel", h.withAuth(h.workerOnly(http.HandlerFunc(h.cancelAttendanceCorrection))))
	}
	mux.Handle("POST /api/v1/schedules/{week}/publish", h.withAuth(h.managerOnly(http.HandlerFunc(h.publish))))
	mux.Handle("GET /api/v1/people", h.withAuth(h.managerOnly(http.HandlerFunc(h.people))))
	mux.Handle("POST /api/v1/people", h.withAuth(h.managerOnly(http.HandlerFunc(h.createEmployee))))
	mux.Handle("PUT /api/v1/people/{employeeID}", h.withAuth(h.managerOnly(http.HandlerFunc(h.updateEmployee))))
	mux.Handle("DELETE /api/v1/people/{employeeID}", h.withAuth(h.managerOnly(http.HandlerFunc(h.deactivateEmployee))))
	mux.Handle("GET /api/v1/audit", h.withAuth(h.managerOnly(http.HandlerFunc(h.audit))))
	return requestLog(securityHeaders(mux))
}

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "time": time.Now().UTC().Format(time.RFC3339)})
}

func (h *Handler) hostAvailability(w http.ResponseWriter, r *http.Request) {
	if !h.config.RequireHostLease {
		writeJSON(w, http.StatusOK, store.HostStatus{Available: true, Instances: []store.HostLease{}})
		return
	}
	status, err := h.store.HostStatus(r.Context())
	if err != nil {
		writeProblem(w, http.StatusServiceUnavailable, "host_status_unavailable", "The Weekline host status is unavailable.")
		return
	}
	if !status.Available {
		writeProblem(w, http.StatusServiceUnavailable, "manager_app_offline", "The employee schedule is offline because no manager app is open.")
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *Handler) renewHostLease(w http.ResponseWriter, r *http.Request) {
	instanceID := r.PathValue("instanceID")
	if !validUUID(instanceID) {
		writeProblem(w, http.StatusBadRequest, "invalid_instance_id", "Instance ID must be a UUID.")
		return
	}
	if !requireJSON(w, r) {
		return
	}
	var input struct {
		MachineName string `json:"machineName"`
		AppVersion  string `json:"appVersion"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	input.MachineName = strings.TrimSpace(input.MachineName)
	input.AppVersion = strings.TrimSpace(input.AppVersion)
	if input.MachineName == "" || utf8.RuneCountInString(input.MachineName) > 120 || utf8.RuneCountInString(input.AppVersion) > 40 {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_host_lease", "Machine name is required and host metadata must fit the configured limits.")
		return
	}
	status, err := h.store.RenewHostLease(r.Context(), store.HostLeaseInput{
		InstanceID: instanceID, MachineName: input.MachineName, AppVersion: input.AppVersion,
	}, h.config.HostLeaseTTL)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "server_error", "Unable to renew the manager host lease.")
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *Handler) releaseHostLease(w http.ResponseWriter, r *http.Request) {
	instanceID := r.PathValue("instanceID")
	if !validUUID(instanceID) {
		writeProblem(w, http.StatusBadRequest, "invalid_instance_id", "Instance ID must be a UUID.")
		return
	}
	status, err := h.store.ReleaseHostLease(r.Context(), instanceID)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "server_error", "Unable to release the manager host lease.")
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *Handler) hostStatus(w http.ResponseWriter, r *http.Request) {
	status, err := h.store.HostStatus(r.Context())
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "server_error", "Unable to load the Weekline host status.")
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	if !requireJSON(w, r) {
		return
	}
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	user, err := h.store.UserByEmail(r.Context(), input.Email)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(input.Password)) != nil {
		writeProblem(w, http.StatusUnauthorized, "invalid_credentials", "Email or password is incorrect.")
		return
	}
	if h.config.RequireHostLease && user.Role == "worker" {
		active, activeErr := h.store.HasActiveHostLease(r.Context())
		if activeErr != nil {
			writeProblem(w, http.StatusServiceUnavailable, "host_status_unavailable", "The Weekline host status is unavailable.")
			return
		}
		if !active {
			writeProblem(w, http.StatusServiceUnavailable, "manager_app_offline", "The employee schedule is offline because no manager app is open.")
			return
		}
	}
	token, err := randomToken()
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "server_error", "Unable to create a session.")
		return
	}
	expires := time.Now().Add(h.config.SessionTTL)
	if err := h.store.CreateSession(r.Context(), token, user.ID, expires); err != nil {
		writeProblem(w, http.StatusInternalServerError, "server_error", "Unable to create a session.")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/", Expires: expires, MaxAge: int(h.config.SessionTTL.Seconds()),
		HttpOnly: true, Secure: h.config.CookieSecure, SameSite: http.SameSiteStrictMode,
	})
	user.PasswordHash = ""
	writeJSON(w, http.StatusOK, user)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookie); err == nil {
		_ = h.store.DeleteSession(r.Context(), cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true,
		Secure: h.config.CookieSecure, SameSite: http.SameSiteStrictMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, currentUser(r))
}

func (h *Handler) getSchedule(w http.ResponseWriter, r *http.Request) {
	week := r.PathValue("week")
	if err := validateWeek(week); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_week", err.Error())
		return
	}
	user := currentUser(r)
	var schedule store.Schedule
	var err error
	if user.Role == "manager" {
		schedule, err = h.store.ManagerSchedule(r.Context(), week)
	} else {
		schedule, err = h.store.WorkerSchedule(r.Context(), week, user.ID)
	}
	if errors.Is(err, store.ErrNotFound) {
		writeProblem(w, http.StatusNotFound, "schedule_not_found", "No published schedule exists for this week.")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "server_error", "Unable to load the schedule.")
		return
	}
	writeJSON(w, http.StatusOK, schedule)
}

func (h *Handler) createShift(w http.ResponseWriter, r *http.Request) {
	if !requireJSON(w, r) {
		return
	}
	week := r.PathValue("week")
	if err := validateWeek(week); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_week", err.Error())
		return
	}
	var input store.ShiftInput
	if err := decodeJSON(r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := validateShift(week, input); err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_shift", err.Error())
		return
	}
	shift, err := h.store.CreateShift(r.Context(), week, currentUser(r).ID, input)
	if h.handleStoreError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, shift)
}

func (h *Handler) evaluateShiftConflicts(w http.ResponseWriter, r *http.Request) {
	if !requireJSON(w, r) {
		return
	}
	week := r.PathValue("week")
	if err := validateWeek(week); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_week", err.Error())
		return
	}
	var input store.ConflictEvaluationInput
	if err := decodeJSON(r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := validateShift(week, input.ShiftInput); err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_shift", err.Error())
		return
	}
	conflicts, err := h.store.EvaluateShiftConflicts(r.Context(), week, input)
	if h.handleStoreError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, conflicts)
}

func (h *Handler) swapDraftShifts(w http.ResponseWriter, r *http.Request) {
	if !requireJSON(w, r) {
		return
	}
	var input store.ShiftSwapInput
	if err := decodeJSON(r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if !validUUID(input.SourceShiftID) || !validUUID(input.TargetShiftID) || input.SourceShiftID == input.TargetShiftID {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_shift_swap", "Choose two different shifts to swap.")
		return
	}
	result, err := h.store.SwapDraftShifts(r.Context(), currentUser(r).ID, input)
	if h.handleStoreError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) updateShift(w http.ResponseWriter, r *http.Request) {
	if !requireJSON(w, r) {
		return
	}
	var input store.ShiftInput
	if err := decodeJSON(r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := validateShiftDateAndTimes(input); err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_shift", err.Error())
		return
	}
	shift, err := h.store.UpdateShift(r.Context(), r.PathValue("shiftID"), currentUser(r).ID, input)
	if h.handleStoreError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, shift)
}

func (h *Handler) deleteShift(w http.ResponseWriter, r *http.Request) {
	if err := h.store.DeleteShift(r.Context(), r.PathValue("shiftID"), currentUser(r).ID); h.handleStoreError(w, err) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) shiftRequests(w http.ResponseWriter, r *http.Request) {
	requests, err := h.store.ListShiftRequests(r.Context(), currentUser(r))
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "server_error", "Unable to load shift requests.")
		return
	}
	writeJSON(w, http.StatusOK, requests)
}

func (h *Handler) swapOptions(w http.ResponseWriter, r *http.Request) {
	sourceShiftID := strings.TrimSpace(r.URL.Query().Get("sourceShiftId"))
	if sourceShiftID == "" {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Source shift is required.")
		return
	}
	options, err := h.store.SwapOptions(r.Context(), currentUser(r).ID, sourceShiftID)
	if h.handleStoreError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, options)
}

func (h *Handler) createSwapRequest(w http.ResponseWriter, r *http.Request) {
	if !requireJSON(w, r) {
		return
	}
	var input store.SwapRequestInput
	if err := decodeJSON(r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := validateSwapRequest(input); err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_shift_request", err.Error())
		return
	}
	user := currentUser(r)
	request, err := h.store.CreateSwapRequest(r.Context(), user.ID, user.ID, input)
	if h.handleStoreError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, request)
}

func (h *Handler) approveShiftRequest(w http.ResponseWriter, r *http.Request) {
	h.resolveShiftRequest(w, r, "approved")
}

func (h *Handler) rejectShiftRequest(w http.ResponseWriter, r *http.Request) {
	h.resolveShiftRequest(w, r, "rejected")
}

func (h *Handler) resolveShiftRequest(w http.ResponseWriter, r *http.Request, decision string) {
	request, err := h.store.ResolveShiftRequest(r.Context(), r.PathValue("requestID"), currentUser(r).ID, decision)
	if h.handleStoreError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, request)
}

func (h *Handler) cancelShiftRequest(w http.ResponseWriter, r *http.Request) {
	request, err := h.store.CancelShiftRequest(r.Context(), r.PathValue("requestID"), currentUser(r).ID)
	if h.handleStoreError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, request)
}

func (h *Handler) publish(w http.ResponseWriter, r *http.Request) {
	week := r.PathValue("week")
	if err := validateWeek(week); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_week", err.Error())
		return
	}
	schedule, err := h.store.Publish(r.Context(), week, currentUser(r).ID)
	if h.handleStoreError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, schedule)
}

func (h *Handler) people(w http.ResponseWriter, r *http.Request) {
	people, err := h.store.ListPeople(r.Context())
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "server_error", "Unable to load people.")
		return
	}
	writeJSON(w, http.StatusOK, people)
}

func (h *Handler) createEmployee(w http.ResponseWriter, r *http.Request) {
	if !requireJSON(w, r) {
		return
	}
	var input store.EmployeeInput
	if err := decodeJSON(r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := validateEmployee(input, true); err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_employee", err.Error())
		return
	}
	employee, err := h.store.CreateEmployee(r.Context(), currentUser(r).ID, input)
	if h.handleStoreError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, employee)
}

func (h *Handler) updateEmployee(w http.ResponseWriter, r *http.Request) {
	if !requireJSON(w, r) {
		return
	}
	var input store.EmployeeInput
	if err := decodeJSON(r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := validateEmployee(input, false); err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_employee", err.Error())
		return
	}
	employee, err := h.store.UpdateEmployee(r.Context(), r.PathValue("employeeID"), currentUser(r).ID, input)
	if h.handleStoreError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, employee)
}

func (h *Handler) deactivateEmployee(w http.ResponseWriter, r *http.Request) {
	employee, err := h.store.DeactivateEmployee(r.Context(), r.PathValue("employeeID"), currentUser(r).ID)
	if h.handleStoreError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, employee)
}

func (h *Handler) audit(w http.ResponseWriter, r *http.Request) {
	events, err := h.store.ListAudit(r.Context(), 100)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "server_error", "Unable to load audit history.")
		return
	}
	writeJSON(w, http.StatusOK, events)
}

func (h *Handler) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookie)
		if err != nil {
			writeProblem(w, http.StatusUnauthorized, "authentication_required", "Sign in to continue.")
			return
		}
		user, err := h.store.UserBySession(r.Context(), cookie.Value)
		if err != nil {
			writeProblem(w, http.StatusUnauthorized, "authentication_required", "Your session has expired.")
			return
		}
		if h.config.RequireHostLease && user.Role == "worker" {
			active, activeErr := h.store.HasActiveHostLease(r.Context())
			if activeErr != nil {
				writeProblem(w, http.StatusServiceUnavailable, "host_status_unavailable", "The Weekline host status is unavailable.")
				return
			}
			if !active {
				writeProblem(w, http.StatusServiceUnavailable, "manager_app_offline", "The employee schedule is offline because no manager app is open.")
				return
			}
		}
		ctx := context.WithValue(r.Context(), userContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (h *Handler) withHostControl(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		configured := []byte(h.config.HostControlToken)
		provided := []byte(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if len(configured) < 32 || len(configured) != len(provided) || subtle.ConstantTimeCompare(configured, provided) != 1 {
			writeProblem(w, http.StatusUnauthorized, "host_control_required", "A valid host-control token is required.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) managerOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if currentUser(r).Role != "manager" {
			writeProblem(w, http.StatusForbidden, "permission_denied", "Manager access is required.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) workerOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if currentUser(r).Role != "worker" {
			writeProblem(w, http.StatusForbidden, "permission_denied", "Worker access is required.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) handleStoreError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, store.ErrNotFound) {
		writeProblem(w, http.StatusNotFound, "not_found", "The requested record was not found.")
		return true
	}
	var conflictError store.ConflictError
	if errors.As(err, &conflictError) {
		message := "The operation has blocking schedule conflicts."
		if len(conflictError.Conflicts) > 0 {
			message = conflictError.Conflicts[0].Message
		}
		writeJSON(w, http.StatusConflict, map[string]any{
			"code": "shift_conflict", "message": message, "conflicts": conflictError.Conflicts,
		})
		return true
	}
	if errors.Is(err, store.ErrConflict) {
		writeProblem(w, http.StatusConflict, "shift_conflict", strings.TrimPrefix(err.Error(), store.ErrConflict.Error()+": "))
		return true
	}
	if errors.Is(err, store.ErrInvalid) {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_shift", strings.TrimPrefix(err.Error(), store.ErrInvalid.Error()+": "))
		return true
	}
	writeProblem(w, http.StatusInternalServerError, "server_error", "The operation could not be completed.")
	return true
}

func currentUser(r *http.Request) store.User {
	user, _ := r.Context().Value(userContextKey).(store.User)
	return user
}

func validateWeek(value string) error {
	week, err := time.Parse("2006-01-02", value)
	if err != nil {
		return errors.New("Week must use YYYY-MM-DD format.")
	}
	if week.Weekday() != time.Monday {
		return errors.New("Week must begin on a Monday.")
	}
	return nil
}

func validUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for index, char := range value {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			if char != '-' {
				return false
			}
			continue
		}
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')) {
			return false
		}
	}
	return true
}

func validateShift(weekValue string, input store.ShiftInput) error {
	if err := validateShiftDateAndTimes(input); err != nil {
		return err
	}
	week, _ := time.Parse("2006-01-02", weekValue)
	date, _ := time.Parse("2006-01-02", input.Date)
	if date.Before(week) || date.After(week.AddDate(0, 0, 6)) {
		return errors.New("Shift date must fall within the selected week.")
	}
	return nil
}

func validateShiftDateAndTimes(input store.ShiftInput) error {
	if strings.TrimSpace(input.EmployeeID) == "" {
		return errors.New("Employee is required.")
	}
	if _, err := time.Parse("2006-01-02", input.Date); err != nil {
		return errors.New("Date must use YYYY-MM-DD format.")
	}
	start, startErr := time.Parse("15:04", input.Start)
	end, endErr := time.Parse("15:04", input.End)
	if startErr != nil || endErr != nil {
		return errors.New("Times must use 24-hour HH:MM format.")
	}
	if !end.After(start) {
		return errors.New("End time must be later than start time.")
	}
	if utf8.RuneCountInString(input.Note) > 120 {
		return errors.New("Notes cannot exceed 120 characters.")
	}
	return nil
}

func validateSwapRequest(input store.SwapRequestInput) error {
	if strings.TrimSpace(input.SourceShiftID) == "" || strings.TrimSpace(input.TargetShiftID) == "" {
		return errors.New("Source and target shifts are required.")
	}
	if input.SourceShiftID == input.TargetShiftID {
		return errors.New("Choose a different shift to swap with.")
	}
	return nil
}

func validateEmployee(input store.EmployeeInput, requirePassword bool) error {
	if strings.TrimSpace(input.DisplayName) == "" {
		return errors.New("Employee name is required.")
	}
	if utf8.RuneCountInString(input.DisplayName) > 80 {
		return errors.New("Employee name cannot exceed 80 characters.")
	}
	email := strings.TrimSpace(input.Email)
	if email == "" {
		return errors.New("Email is required.")
	}
	if !strings.Contains(email, "@") || strings.ContainsAny(email, " \t\r\n") {
		return errors.New("Email must be valid.")
	}
	if requirePassword && len(input.InitialPassword) < 8 {
		return errors.New("Initial password must be at least 8 characters.")
	}
	if !requirePassword && input.InitialPassword != "" {
		return errors.New("Initial password can only be set when creating an employee.")
	}
	status := strings.TrimSpace(input.Status)
	if status == "" {
		status = "active"
	}
	if status != "active" && status != "inactive" && status != "deactivated" {
		return errors.New("Employee status must be active, inactive, or deactivated.")
	}
	if input.ScheduleEligible != nil && *input.ScheduleEligible && status != "active" {
		return errors.New("Only active employees can be schedule eligible.")
	}
	return nil
}

func decodeJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("Invalid JSON: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("Request body must contain one JSON object.")
	}
	return nil
}

func requireJSON(w http.ResponseWriter, r *http.Request) bool {
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		writeProblem(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "Content-Type must be application/json.")
		return false
	}
	return true
}

func randomToken() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { next.ServeHTTP(w, r) })
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeProblem(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]string{"code": code, "message": message})
}
