package booking

import "strconv"

// Movie describes a movie's catalog entry and screen layout.
type Movie struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Rows        int    `json:"rows"`
	SeatsPerRow int    `json:"seats_per_row"`
	PriceCents  int    `json:"price_cents"`
}

// Catalog provides read access to the movie catalog.
type Catalog struct {
	movies []Movie
}

// NewCatalog creates a catalog from the supplied movies.
func NewCatalog(movies []Movie) *Catalog {
	return &Catalog{movies: movies}
}

// All returns every movie in the catalog.
func (c *Catalog) All() []Movie {
	return c.movies
}

// Get returns the movie with the given ID, if present.
func (c *Catalog) Get(id string) (Movie, bool) {
	for _, m := range c.movies {
		if m.ID == id {
			return m, true
		}
	}
	return Movie{}, false
}

const rowLetters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"

// seatLabel builds the canonical ID for a seat at the given position.
// rowIndex is zero-based, seatNumber is one-based (e.g. A1).
func seatLabel(rowIndex, seatNumber int) string {
	return string(rowLetters[rowIndex]) + strconv.Itoa(seatNumber)
}

// HasSeat reports whether seatID (e.g. "B7") exists in the movie's layout.
func (m Movie) HasSeat(seatID string) bool {
	if len(seatID) < 2 {
		return false
	}
	row := int(seatID[0] - 'A')
	n, err := strconv.Atoi(seatID[1:])
	if err != nil || row < 0 || row >= m.Rows || n < 1 || n > m.SeatsPerRow {
		return false
	}
	return seatLabel(row, n) == seatID // rejects non-canonical forms like "A01"
}
