export interface Movie {
  id: string
  title: string
  rows: number
  seats_per_row: number
  price_cents: number
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

const API_BASE = import.meta.env.VITE_API_BASE ?? ''

let tokenGetter: (() => Promise<string | null>) | null = null

export function setTokenGetter(
  getter: (() => Promise<string | null>) | null,
): void {
  tokenGetter = getter
}

export class ApiError extends Error {
  status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  if (tokenGetter) {
    const token = await tokenGetter()
    if (token) headers['Authorization'] = `Bearer ${token}`
  }
  const res = await fetch(`${API_BASE}${path}`, {
    headers,
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

export function holdSeat(movieID: string, seatID: string): Promise<HoldResponse> {
  return request<HoldResponse>(
    `/movies/${encodeURIComponent(movieID)}/seats/${encodeURIComponent(seatID)}/hold`,
    { method: 'POST' },
  )
}

export interface CheckoutResponse {
  url: string
  id: string
}

export interface CheckoutRequest {
  session_ids: string[]
  success_url: string
  cancel_url: string
}

export function createCheckout(req: CheckoutRequest): Promise<CheckoutResponse> {
  return request<CheckoutResponse>(`/sessions/checkout`, {
    method: 'POST',
    body: JSON.stringify(req),
  })
}

export function releaseSession(sessionID: string): Promise<void> {
  return request<void>(`/sessions/${encodeURIComponent(sessionID)}`, {
    method: 'DELETE',
  })
}
