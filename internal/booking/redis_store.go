package booking

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const defaultHoldTTL = 2 * time.Minute

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
	pattern := fmt.Sprintf("seat:%s:*", movieID)
	var sessions []Booking

	ctx := context.Background()

	iter := s.rdb.Scan(ctx, 0, pattern, 0).Iterator()
	for iter.Next(ctx) {
		val, err := s.rdb.Get(ctx, iter.Val()).Result()
		if err != nil {
			continue
		}

		session, err := parseSession(val)
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
	now := time.Now()
	ctx := context.Background()
	key := fmt.Sprintf("seat:%s:%s", b.MovieID, b.SeatID)

	b.ID = id
	b.Status = "held"
	b.ExpiresAt = now.Add(defaultHoldTTL)
	val, _ := json.Marshal(b)

	res := s.rdb.SetArgs(ctx, key, val, redis.SetArgs{
		Mode: "NX",
		TTL:  defaultHoldTTL,
	})

	ok := res.Val() == "OK"
	if !ok {
		return Booking{}, ErrSeatAlreadyBooked
	}

	s.rdb.Set(ctx, sessionKey(id), key, defaultHoldTTL)

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

// Confirm converts a held session into a permanent booking and removes the TTL.
func (s *RedisStore) Confirm(ctx context.Context, sessionID string, userID string) (Booking, error) {
	session, sk, err := s.getSession(ctx, sessionID, userID)
	if err != nil {
		return Booking{}, err
	}

	s.rdb.Persist(ctx, sk)
	s.rdb.Persist(ctx, sessionKey(sessionID))

	session.Status = "confirmed"
	session.ExpiresAt = time.Time{}

	val, _ := json.Marshal(session)
	s.rdb.Set(ctx, sk, val, 0)

	return session, nil
}

// getSession resolves a session ID to its stored booking payload.
func (s *RedisStore) getSession(ctx context.Context, sessionID string, userID string) (Booking, string, error) {
	sk, err := s.rdb.Get(ctx, sessionKey(sessionID)).Result()
	if err != nil {
		if err == redis.Nil {
			return Booking{}, "", ErrSessionNotFound
		}
		return Booking{}, "", err
	}

	val, err := s.rdb.Get(ctx, sk).Result()
	if err != nil {
		if err == redis.Nil {
			return Booking{}, "", ErrSessionNotFound
		}
		return Booking{}, "", err
	}

	session, err := parseSession(val)
	if err != nil {
		return Booking{}, "", err
	}

	if session.UserID != userID {
		return Booking{}, "", ErrUnauthorized
	}

	return session, sk, nil
}

// Release removes an active hold and clears the reverse lookup entry.
func (s *RedisStore) Release(ctx context.Context, sessionID string, userID string) error {
	_, sk, err := s.getSession(ctx, sessionID, userID)
	if err != nil {
		return err
	}

	s.rdb.Del(ctx, sk, sessionKey(sessionID))
	return nil
}
