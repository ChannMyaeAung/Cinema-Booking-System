import { Link } from 'react-router-dom'
import { useMovies } from '../api/queries'
import type { Movie } from '../api/api'

function posterHue(id: string): number {
  let hash = 0
  for (const ch of id) hash = (hash * 31 + ch.charCodeAt(0)) % 360
  return hash
}

function MovieCard({ movie }: { movie: Movie }) {
  const hue = posterHue(movie.id)
  return (
    <Link to={`/movie/${encodeURIComponent(movie.id)}`} className="movie-card">
      <div
        className="movie-poster"
        style={{
          background: `linear-gradient(150deg, hsl(${hue} 55% 32%), hsl(${(hue + 40) % 360} 60% 14%))`,
        }}
        aria-hidden="true"
      >
        {movie.title.charAt(0).toUpperCase()}
      </div>
      <div className="movie-info">
        <h3 className="movie-title">{movie.title}</h3>
        <span className="movie-meta">
          {movie.rows} rows · {movie.seats_per_row} seats per row
        </span>
        <button type="button" className="movie-action">
          Book seats
        </button>
      </div>
    </Link>
  )
}

export default function Home() {
  const { data: movies, isPending, isError, error } = useMovies()

  return (
    <>
      <h1 className="page-title">Now showing</h1>
      <p className="page-subtitle">Pick a film to choose your seats.</p>

      {isPending && (
        <div className="movie-grid" aria-hidden="true">
          {Array.from({ length: 4 }).map((_, i) => (
            <div className="movie-card" key={i}>
              <div className="movie-poster movie-poster-skeleton" />
              <div className="movie-info">
                <div className="skeleton-line" />
                <div className="skeleton-line short" />
              </div>
            </div>
          ))}
        </div>
      )}

      {isError && (
        <div className="error-banner">
          Could not load the movie list:{' '}
          {error instanceof Error ? error.message : 'unknown error'}
        </div>
      )}

      {movies && (
        <div className="movie-grid">
          {movies.map((movie) => (
            <MovieCard key={movie.id} movie={movie} />
          ))}
        </div>
      )}
    </>
  )
}
