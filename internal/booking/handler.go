package booking

import (
	"cinema-booking-system/internal/auth"
	"cinema-booking-system/internal/payment"
	"cinema-booking-system/internal/utils"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"time"
)

// handler exposes booking-related HTTP endpoints.
type handler struct {
	svc     *Service
	catalog *Catalog
	pay     payment.PaymentGateway
}

// NewHandler creates a new booking handler with the supplied service, catalog,
// and payment gateway.
func NewHandler(svc *Service, catalog *Catalog, pay payment.PaymentGateway) *handler {
	return &handler{svc: svc, catalog: catalog, pay: pay}
}

// HoldSeat creates a temporary seat hold for the provided movie and seat.
func (h *handler) HoldSeat(w http.ResponseWriter, r *http.Request) {
	movieID := r.PathValue("movieID")
	seatID := r.PathValue("seatID")

	userID := auth.UserID(r.Context())
	if userID == "" {
		utils.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	data := Booking{
		UserID:  userID,
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

// checkoutRequest is the body for creating a payment checkout.
type checkoutRequest struct {
	SessionIDs []string `json:"session_ids"`
	SuccessURL string   `json:"success_url"`
	CancelURL  string   `json:"cancel_url"`
}

// CreateCheckout creates a payment checkout session for held seats. The seats
// are NOT confirmed here — confirmation happens only in the Stripe webhook
// handler after payment succeeds (never trust the client saying "paid").
func (h *handler) CreateCheckout(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserID(r.Context())
	if userID == "" {
		utils.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req checkoutRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		utils.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.SessionIDs) == 0 {
		utils.WriteError(w, http.StatusBadRequest, "session_ids is required")
		return
	}
	if req.SuccessURL == "" || req.CancelURL == "" {
		utils.WriteError(w, http.StatusBadRequest, "success_url and cancel_url are required")
		return
	}

	var lineItems []payment.LineItem
	for _, sessionID := range req.SessionIDs {
		session, err := h.svc.GetSession(r.Context(), sessionID, userID)
		if err != nil {
			switch {
			case errors.Is(err, ErrSessionNotFound):
				utils.WriteError(w, http.StatusNotFound, err.Error())
			case errors.Is(err, ErrUnauthorized):
				utils.WriteError(w, http.StatusForbidden, err.Error())
			default:
				utils.WriteError(w, http.StatusInternalServerError, "failed to load session")
			}
			return
		}

		movie, ok := h.catalog.Get(session.MovieID)
		if !ok {
			utils.WriteError(w, http.StatusInternalServerError, "movie not found")
			return
		}

		lineItems = append(lineItems, payment.LineItem{
			Name:   movie.Title + " — Seat " + session.SeatID,
			Amount: int64(movie.PriceCents),
			Qty:    1,
		})
	}

	cs, err := h.pay.CreateCheckoutSession(r.Context(), payment.CheckoutParams{
		SessionIDs: req.SessionIDs,
		UserID:     userID,
		LineItems:  lineItems,
		SuccessURL: req.SuccessURL,
		CancelURL:  req.CancelURL,
	})
	if err != nil {
		log.Printf("creating checkout for %v: %v", req.SessionIDs, err)
		utils.WriteError(w, http.StatusInternalServerError, "failed to create checkout")
		return
	}

	// Payment adds time to the flow; extend the holds so they survive checkout.
	for _, sessionID := range req.SessionIDs {
		if err := h.svc.ExtendHold(r.Context(), sessionID, userID, checkoutHoldTTL); err != nil {
			log.Printf("extending hold for %s: %v", sessionID, err)
		}
	}

	utils.WriteJSON(w, http.StatusCreated, map[string]string{
		"url": cs.URL,
		"id":  cs.ID,
	})
}

// StripeWebhook receives signature-verified Stripe events. Seats are confirmed
// ONLY here — a checkout.session.completed event is the single trusted signal
// that payment succeeded. Confirming an already-confirmed session is a no-op.
func (h *handler) StripeWebhook(w http.ResponseWriter, r *http.Request) {
	payload, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		utils.WriteError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	signature := r.Header.Get("Stripe-Signature")

	event, err := h.pay.ParseWebhookEvent(r.Context(), payload, signature)
	if err != nil {
		utils.WriteError(w, http.StatusBadRequest, "invalid webhook signature")
		return
	}

	if event.Type == "checkout.session.completed" {
		for _, sessionID := range event.SessionIDs {
			if _, err := h.svc.ConfirmSeat(r.Context(), sessionID, event.UserID); err != nil {
				log.Printf("webhook confirm %s: %v", sessionID, err)
			}
		}
	}

	w.WriteHeader(http.StatusOK)
}

// ReleaseSession removes an active seat hold for the specified session.
func (h *handler) ReleaseSession(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionID")

	userID := auth.UserID(r.Context())
	if userID == "" {
		utils.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	err := h.svc.ReleaseSeat(r.Context(), sessionID, userID)
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
