export interface Movie {
  id: string
  title: string
  rows: number
  seats_per_row: number
}

export interface SeatInfo {
  seat_id: string
  user_id: string
  booked: boolean
  confirmed: boolean
}

export interface HoldResponse {
  session_id: string
  movie_id: string
  seat_id: string
  expires_at: string
}

export type HoldInfo = HoldResponse

export interface SessionResponse {
  session_id: string
  movie_id: string
  seat_id: string
  user_id: string
  status: string
  expires_at: string
}

const API_BASE = import.meta.env.VITE_API_BASE ?? ''

export class ApiError extends Error {
  status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, {
    headers: { 'Content-Type': 'application/json' },
    ...init,
  })

  if (res.status === 204) return undefined as T

  let body: unknown = null
  const text = await res.text()
  if (text) {
    try {
      body = JSON.parse(text)
    } catch {
      body = null
    }
  }

  if (!res.ok) {
    const message =
      body && typeof body === 'object' && 'error' in body
        ? String((body as { error: unknown }).error)
        : res.statusText
    throw new ApiError(res.status, message)
  }

  return body as T
}

export function listMovies(): Promise<Movie[]> {
  return request<Movie[]>('/movies')
}

export function listSeats(movieID: string): Promise<SeatInfo[]> {
  return request<SeatInfo[]>(`/movies/${encodeURIComponent(movieID)}/seats`)
}

export function holdSeat(
  movieID: string,
  seatID: string,
  userID: string,
): Promise<HoldResponse> {
  return request<HoldResponse>(
    `/movies/${encodeURIComponent(movieID)}/seats/${encodeURIComponent(seatID)}/hold`,
    { method: 'POST', body: JSON.stringify({ user_id: userID }) },
  )
}

export function confirmSession(
  sessionID: string,
  userID: string,
): Promise<SessionResponse> {
  return request<SessionResponse>(
    `/sessions/${encodeURIComponent(sessionID)}/confirm`,
    { method: 'PUT', body: JSON.stringify({ user_id: userID }) },
  )
}

export function releaseSession(sessionID: string, userID: string): Promise<void> {
  return request<void>(`/sessions/${encodeURIComponent(sessionID)}`, {
    method: 'DELETE',
    body: JSON.stringify({ user_id: userID }),
  })
}
