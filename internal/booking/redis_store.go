package booking

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	defaultHoldTTL = 2 * time.Minute
	// checkoutSessionTTL is how long a Stripe Checkout page stays payable.
	// Stripe requires at least 30 minutes; the extra minute absorbs clock skew.
	checkoutSessionTTL = 31 * time.Minute
	// checkoutHoldTTL outlives the Checkout page so a payment completed at the
	// last moment still finds its hold when the webhook arrives.
	checkoutHoldTTL = 35 * time.Minute
)

// maxTxRetries bounds optimistic-transaction retries when a watched seat key
// changes mid-update (e.g. the hold expires while being confirmed).
const maxTxRetries = 3

// RedisStore provides Redis-backed persistence for seat booking sessions.
//
// Key layout:
//   - seat:{movieID}:{seatID} -> booking session payload
//   - session:{sessionID} -> reverse lookup for the seat key
type RedisStore struct {
	rdb *redis.Client
}

// NewRedisStore creates a Redis-backed booking store.
func NewRedisStore(rdb *redis.Client) *RedisStore {
	return &RedisStore{rdb}
}

// sessionKey returns the Redis key used for session-to-seat lookup.
func sessionKey(id string) string {
	return fmt.Sprintf("session:%s", id)
}

// seatKey returns the Redis key holding a seat's booking payload.
func seatKey(movieID, seatID string) string {
	return fmt.Sprintf("seat:%s:%s", movieID, seatID)
}

// Book places a seat on hold and returns the resulting booking session.
func (s *RedisStore) Book(b Booking) (Booking, error) {
	session, err := s.hold(b)
	if err != nil {
		return Booking{}, err
	}

	log.Printf("session booked: %+v", session)
	return session, nil
}

// ListBookings returns all active booking sessions for the specified movie.
func (s *RedisStore) ListBookings(movieID string) []Booking {
	ctx := context.Background()

	var keys []string
	iter := s.rdb.Scan(ctx, 0, seatKey(movieID, "*"), 0).Iterator()
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}
	if len(keys) == 0 {
		return nil
	}

	// One round trip for every seat instead of a GET per key.
	vals, err := s.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		log.Printf("listing bookings for %s: %v", movieID, err)
		return nil
	}

	sessions := make([]Booking, 0, len(vals))
	for _, v := range vals {
		str, ok := v.(string) // nil when the key expired between SCAN and MGET
		if !ok {
			continue
		}
		session, err := parseSession(str)
		if err != nil {
			continue
		}
		sessions = append(sessions, session)
	}
	return sessions
}

// hold attempts to reserve the seat with a temporary TTL.
func (s *RedisStore) hold(b Booking) (Booking, error) {
	id := uuid.New().String()
	ctx := context.Background()
	key := seatKey(b.MovieID, b.SeatID)

	b.ID = id
	b.Status = "held"
	b.ExpiresAt = time.Now().Add(defaultHoldTTL)
	val, _ := json.Marshal(b)

	ok, err := s.rdb.SetNX(ctx, key, val, defaultHoldTTL).Result()
	if err != nil {
		return Booking{}, err
	}
	if !ok {
		return Booking{}, ErrSeatAlreadyBooked
	}

	if err := s.rdb.Set(ctx, sessionKey(id), key, defaultHoldTTL).Err(); err != nil {
		s.rdb.Del(ctx, key) // don't leave an unreachable hold behind
		return Booking{}, err
	}

	return b, nil
}

// parseSession converts the stored JSON payload into a Booking value.
func parseSession(val string) (Booking, error) {
	var data Booking
	if err := json.Unmarshal([]byte(val), &data); err != nil {
		return Booking{}, err
	}

	return data, nil
}

// getSession resolves a session ID to its stored booking payload and seat key.
func (s *RedisStore) getSession(ctx context.Context, sessionID string, userID string) (Booking, string, error) {
	sk, err := s.rdb.Get(ctx, sessionKey(sessionID)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return Booking{}, "", ErrSessionNotFound
		}
		return Booking{}, "", err
	}

	val, err := s.rdb.Get(ctx, sk).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return Booking{}, "", ErrSessionNotFound
		}
		return Booking{}, "", err
	}

	session, err := parseSession(val)
	if err != nil {
		return Booking{}, "", err
	}
	// The seat may have been re-held under a new session since this one lapsed.
	if session.ID != sessionID {
		return Booking{}, "", ErrSessionNotFound
	}
	if session.UserID != userID {
		return Booking{}, "", ErrUnauthorized
	}

	return session, sk, nil
}

// updateSession runs fn inside an optimistic transaction that WATCHes the
// seat key, so the read-check-write is atomic with respect to hold expiry,
// another customer re-holding the seat, and staff voids. fn receives the
// current booking (already verified to belong to sessionID) and queues its
// writes on the pipeliner.
func (s *RedisStore) updateSession(ctx context.Context, sessionID string, fn func(b Booking, sk string, pipe redis.Pipeliner) error) error {
	sk, err := s.rdb.Get(ctx, sessionKey(sessionID)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return ErrSessionNotFound
		}
		return err
	}

	txf := func(tx *redis.Tx) error {
		val, err := tx.Get(ctx, sk).Result()
		if err != nil {
			if errors.Is(err, redis.Nil) {
				return ErrSessionNotFound
			}
			return err
		}
		b, err := parseSession(val)
		if err != nil {
			return err
		}
		if b.ID != sessionID {
			return ErrSessionNotFound
		}

		var fnErr error
		_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			fnErr = fn(b, sk, pipe)
			return fnErr
		})
		if fnErr != nil {
			return fnErr
		}
		return err
	}

	for range maxTxRetries {
		err = s.rdb.Watch(ctx, txf, sk)
		if !errors.Is(err, redis.TxFailedErr) {
			return err
		}
	}
	return err
}

// GetSession returns the booking payload for a session owned by userID.
func (s *RedisStore) GetSession(ctx context.Context, sessionID string, userID string) (Booking, error) {
	session, _, err := s.getSession(ctx, sessionID, userID)
	return session, err
}

// Confirm converts a held session into a permanent booking and removes the TTL.
// Confirming an already-confirmed session is a no-op (idempotent).
func (s *RedisStore) Confirm(ctx context.Context, sessionID string, userID string) (Booking, error) {
	var out Booking
	err := s.updateSession(ctx, sessionID, func(b Booking, sk string, pipe redis.Pipeliner) error {
		if b.UserID != userID {
			return ErrUnauthorized
		}
		out = b
		if b.Status == "confirmed" {
			return nil
		}

		out.Status = "confirmed"
		out.ExpiresAt = time.Time{}
		val, _ := json.Marshal(out)
		pipe.Set(ctx, sk, val, 0)
		pipe.Persist(ctx, sessionKey(sessionID))
		return nil
	})
	if err != nil {
		return Booking{}, err
	}
	return out, nil
}

// ExtendHold refreshes the TTL on a held session (used when a payment
// checkout begins, so the reservation survives the payment flow).
func (s *RedisStore) ExtendHold(ctx context.Context, sessionID string, userID string, ttl time.Duration) error {
	return s.updateSession(ctx, sessionID, func(b Booking, sk string, pipe redis.Pipeliner) error {
		if b.UserID != userID {
			return ErrUnauthorized
		}
		if b.Status == "confirmed" {
			return nil
		}

		b.ExpiresAt = time.Now().Add(ttl)
		val, _ := json.Marshal(b)
		pipe.Set(ctx, sk, val, ttl)
		pipe.Expire(ctx, sessionKey(sessionID), ttl)
		return nil
	})
}

// Release removes an active hold and clears the reverse lookup entry.
func (s *RedisStore) Release(ctx context.Context, sessionID string, userID string) error {
	return s.updateSession(ctx, sessionID, func(b Booking, sk string, pipe redis.Pipeliner) error {
		if b.UserID != userID {
			return ErrUnauthorized
		}
		// A confirmed booking is permanent; never delete it via release.
		if b.Status == "confirmed" {
			return nil
		}

		pipe.Del(ctx, sk, sessionKey(sessionID))
		return nil
	})
}

// AdminCancel removes a booking (held or confirmed) so the seat can be booked
// again. Only existence is checked — there is no ownership requirement.
func (s *RedisStore) AdminCancel(ctx context.Context, sessionID string) error {
	return s.updateSession(ctx, sessionID, func(b Booking, sk string, pipe redis.Pipeliner) error {
		pipe.Del(ctx, sk, sessionKey(sessionID))
		return nil
	})
}
