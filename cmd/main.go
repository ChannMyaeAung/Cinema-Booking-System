package main

import (
	"cinema-booking-system/internal/adapters/redis"
	"cinema-booking-system/internal/auth"
	"cinema-booking-system/internal/booking"
	"cinema-booking-system/internal/payment"
	"cinema-booking-system/internal/utils"
	"context"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/clerk/clerk-sdk-go/v2"
	"github.com/joho/godotenv"
)

// main starts the HTTP server and wires the booking application routes.
func main() {
	// Load root .env if present; a missing file is fine, a malformed one is not.
	if err := godotenv.Load(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		log.Fatalf("loading .env: %v", err)
	}

	secretKey := os.Getenv("CLERK_SECRET_KEY")
	if secretKey == "" {
		log.Fatal("CLERK_SECRET_KEY environment variable is required")
	}
	clerk.SetKey(secretKey)

	stripeSecretKey := os.Getenv("STRIPE_SECRET_KEY")
	if stripeSecretKey == "" {
		log.Fatal("STRIPE_SECRET_KEY environment variable is required")
	}
	stripeWebhookSecret := os.Getenv("STRIPE_WEBHOOK_SECRET")
	if stripeWebhookSecret == "" {
		log.Fatal("STRIPE_WEBHOOK_SECRET environment variable is required")
	}

	mux := http.NewServeMux()

	catalog := booking.NewCatalog(movies)
	mux.HandleFunc("GET /movies", listMovies(catalog))

	// Serve the built React client at root. In dev, use the Vite server on
	// :5173 instead (it proxies API calls here).
	rootDir := "static/client/dist"
	if _, err := os.Stat(filepath.Join(rootDir, "index.html")); err != nil {
		log.Printf("client not built (%s missing); run `pnpm build` in static/client or use the Vite dev server", rootDir)
	}
	mux.Handle("GET /", http.FileServer(http.Dir(rootDir)))

	rdb, err := redis.NewClient(envOr("REDIS_ADDR", "localhost:6379"))
	if err != nil {
		log.Fatalf("redis: %v", err)
	}
	store := booking.NewRedisStore(rdb)
	svc := booking.NewService(store)
	pay := payment.NewStripeGateway(stripeSecretKey, stripeWebhookSecret)
	bookingHandler := booking.NewHandler(svc, catalog, pay)

	mux.HandleFunc("GET /movies/{movieID}/seats", bookingHandler.ListSeats)
	mux.Handle("POST /movies/{movieID}/seats/{seatID}/hold", auth.Middleware(http.HandlerFunc(bookingHandler.HoldSeat)))
	mux.Handle("POST /sessions/checkout", auth.Middleware(http.HandlerFunc(bookingHandler.CreateCheckout)))
	mux.HandleFunc("POST /stripe/webhook", bookingHandler.StripeWebhook)
	mux.Handle("DELETE /sessions/{sessionID}", auth.Middleware(http.HandlerFunc(bookingHandler.ReleaseSession)))

	admin := auth.AdminMiddleware(auth.ClerkRoleChecker())
	mux.Handle("POST /admin/sessions/confirm", admin(http.HandlerFunc(bookingHandler.AdminConfirmSeats)))
	mux.Handle("DELETE /admin/sessions/{sessionID}", admin(http.HandlerFunc(bookingHandler.AdminCancelSession)))

	server := &http.Server{
		Addr:              ":" + envOr("PORT", "8080"),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Printf("listening on %s", server.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	// SIGTERM is what Docker/Kubernetes send on stop; SIGINT is Ctrl+C.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	log.Print("shutting down server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("forced shutdown: %v", err)
	}
	if err := rdb.Close(); err != nil {
		log.Printf("closing redis: %v", err)
	}
}

// envOr returns the environment variable key, or fallback when it is unset.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// movies contains the sample catalog exposed by the API.
var movies = []booking.Movie{
	{ID: "inception", Title: "Inception", Rows: 5, SeatsPerRow: 8, PriceCents: 1500},
	{ID: "dune", Title: "Dune: Part Two", Rows: 4, SeatsPerRow: 6, PriceCents: 1200},
}

// listMovies returns the available movie catalog as JSON.
func listMovies(catalog *booking.Catalog) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		utils.WriteJSON(w, http.StatusOK, catalog.All())
	}
}
