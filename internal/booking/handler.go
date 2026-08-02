package booking

import (
	"cinema-booking-system/internal/utils"
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

// handler exposes booking-related HTTP endpoints.
type handler struct {
	svc     *Service
	catalog *Catalog
}

// NewHandler creates a new booking handler with the supplied service and catalog.
func NewHandler(svc *Service, catalog *Catalog) *handler {
	return &handler{svc: svc, catalog: catalog}
}

// holdSeatRequest represents the payload expected for a seat hold request.
type holdSeatRequest struct {
	UserID string `json:"user_id"`
}

// HoldSeat creates a temporary seat hold for the provided movie and seat.
func (h *handler) HoldSeat(w http.ResponseWriter, r *http.Request) {
	movieID := r.PathValue("movieID")
	seatID := r.PathValue("seatID")

	var req holdSeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.UserID == "" {
		utils.WriteError(w, http.StatusBadRequest, "user_id is required")
		return
	}

	data := Booking{
		UserID:  req.UserID,
		SeatID:  seatID,
		MovieID: movieID,
	}

	// Attempt to hold the seat using the booking service
	session, err := h.svc.Book(data)
	if err != nil {
		if errors.Is(err, ErrSeatAlreadyBooked) {
			utils.WriteError(w, http.StatusConflict, err.Error())
			return
		}
		utils.WriteError(w, http.StatusInternalServerError, "failed to hold seat")
		return
	}

	type holdResponse struct {
		SessionID string `json:"session_id"`
		MovieID   string `json:"movie_id"`
		SeatID    string `json:"seat_id"`
		ExpiresAt string `json:"expires_at"`
	}

	utils.WriteJSON(w, http.StatusCreated, holdResponse{
		SeatID:    seatID,
		MovieID:   session.MovieID,
		SessionID: session.ID,
		ExpiresAt: session.ExpiresAt.Format(time.RFC3339),
	})
}

// ListSeats returns the full seat grid with the current state for each seat.
func (h *handler) ListSeats(w http.ResponseWriter, r *http.Request) {
	movieID := r.PathValue("movieID")

	movie, ok := h.catalog.Get(movieID)
	if !ok {
		utils.WriteError(w, http.StatusNotFound, "movie not found")
		return
	}

	state := make(map[string]seatInfo)
	for _, b := range h.svc.ListBookings(movieID) {
		state[b.SeatID] = seatInfo{
			SeatID:    b.SeatID,
			UserID:    b.UserID,
			Booked:    true,
			Confirmed: b.Status == "confirmed",
		}
	}

	seats := make([]seatInfo, 0, movie.Rows*movie.SeatsPerRow)
	for row := 0; row < movie.Rows; row++ {
		for n := 1; n <= movie.SeatsPerRow; n++ {
			id := seatLabel(row, n)
			info, exists := state[id]
			if !exists {
				info = seatInfo{SeatID: id}
			}
			seats = append(seats, info)
		}
	}

	utils.WriteJSON(w, http.StatusOK, seats)
}

// seatInfo describes the booking state for a single seat.
type seatInfo struct {
	SeatID    string `json:"seat_id"`
	UserID    string `json:"user_id"`
	Booked    bool   `json:"booked"`
	Confirmed bool   `json:"confirmed"`
}

// ConfirmSession confirms a previously held seat session.
func (h *handler) ConfirmSession(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionID")

	var req holdSeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.UserID == "" {
		utils.WriteError(w, http.StatusBadRequest, "user_id is required")
		return
	}

	session, err := h.svc.ConfirmSeat(r.Context(), sessionID, req.UserID)
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			utils.WriteError(w, http.StatusNotFound, err.Error())
			return
		}
		if errors.Is(err, ErrUnauthorized) {
			utils.WriteError(w, http.StatusForbidden, err.Error())
			return
		}
		utils.WriteError(w, http.StatusInternalServerError, "failed to confirm session")
		return
	}

	utils.WriteJSON(w, http.StatusOK, sessionResponse{
		SessionID: session.ID,
		MovieID:   session.MovieID,
		SeatID:    session.SeatID,
		UserID:    req.UserID,
		Status:    session.Status,
		ExpiresAt: session.ExpiresAt.Format(time.RFC3339),
	})
}

// sessionResponse represents the response payload for a confirmed or released session.
type sessionResponse struct {
	SessionID string `json:"session_id"`
	MovieID   string `json:"movie_id"`
	SeatID    string `json:"seat_id"`
	UserID    string `json:"user_id"`
	Status    string `json:"status"`
	ExpiresAt string `json:"expires_at"`
}

// ReleaseSession removes an active seat hold for the specified session.
func (h *handler) ReleaseSession(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionID")

	var req holdSeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.UserID == "" {
		utils.WriteError(w, http.StatusBadRequest, "user_id is required")
		return
	}

	err := h.svc.ReleaseSeat(r.Context(), sessionID, req.UserID)
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			utils.WriteError(w, http.StatusNotFound, err.Error())
			return
		}
		if errors.Is(err, ErrUnauthorized) {
			utils.WriteError(w, http.StatusForbidden, err.Error())
			return
		}
		utils.WriteError(w, http.StatusInternalServerError, "failed to release session")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
