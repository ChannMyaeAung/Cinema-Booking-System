package main

import (
	"cinema-booking-system/internal/adapters/redis"
	"cinema-booking-system/internal/booking"
	"cinema-booking-system/internal/utils"
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"
)

// main starts the HTTP server and wires the booking application routes.
func main() {
	mux := http.NewServeMux()

	catalog := booking.NewCatalog(movies)
	mux.HandleFunc("GET /movies", listMovies(catalog))
	mux.Handle("GET /", http.FileServer(http.Dir("static")))

	rdb, err := redis.NewClient("localhost:6379")
	if err != nil {
		log.Fatalf("redis: %v", err)
	}
	store := booking.NewRedisStore(rdb)
	svc := booking.NewService(store)
	bookingHandler := booking.NewHandler(svc, catalog)

	mux.HandleFunc("GET /movies/{movieID}/seats", bookingHandler.ListSeats)
	mux.HandleFunc("POST /movies/{movieID}/seats/{seatID}/hold", bookingHandler.HoldSeat)
	mux.HandleFunc("PUT /sessions/{sessionID}/confirm", bookingHandler.ConfirmSession)
	mux.HandleFunc("DELETE /sessions/{sessionID}", bookingHandler.ReleaseSession)

	server := &http.Server{Addr: ":8080", Handler: mux}

	go func() {
		log.Printf("listening on :8080")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit

	log.Print("shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("forced shutdown: %v", err)
	}
}

// movies contains the sample catalog exposed by the API.
var movies = []booking.Movie{
	{ID: "inception", Title: "Inception", Rows: 5, SeatsPerRow: 8},
	{ID: "dune", Title: "Dune: Part Two", Rows: 4, SeatsPerRow: 6},
}

// listMovies returns the available movie catalog as JSON.
func listMovies(catalog *booking.Catalog) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		utils.WriteJSON(w, http.StatusOK, catalog.All())
	}
}
