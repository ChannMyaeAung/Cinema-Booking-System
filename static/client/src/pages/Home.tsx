import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import * as api from '../api/api'

type MoviesState =
  | { status: 'loading' }
  | { status: 'error'; message: string }
  | { status: 'ready'; movies: api.Movie[] }

function posterHue(id: string): number {
  let hash = 0
  for (const ch of id) hash = (hash * 31 + ch.charCodeAt(0)) % 360
  return hash
}

function MovieCard({ movie }: { movie: api.Movie }) {
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
  const [state, setState] = useState<MoviesState>({ status: 'loading' })

  useEffect(() => {
    let active = true
    api
      .listMovies()
      .then((movies) => {
        if (active) setState({ status: 'ready', movies })
      })
      .catch((err: unknown) => {
        if (active) {
          setState({
            status: 'error',
            message: err instanceof Error ? err.message : 'Failed to load movies',
          })
        }
      })
    return () => {
      active = false
    }
  }, [])

  return (
    <>
      <h1 className="page-title">Now showing</h1>
      <p className="page-subtitle">Pick a film to choose your seats.</p>

      {state.status === 'loading' && <div className="spinner" />}

      {state.status === 'error' && (
        <div className="error-banner">
          Could not load the movie list: {state.message}
        </div>
      )}

      {state.status === 'ready' && (
        <div className="movie-grid">
          {state.movies.map((movie) => (
            <MovieCard key={movie.id} movie={movie} />
          ))}
        </div>
      )}
    </>
  )
}
