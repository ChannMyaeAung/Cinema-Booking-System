import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import * as api from './api'

export const HOLDS_MS = 2 * 60 * 1000

export function useMovies() {
  return useQuery({
    queryKey: ['movies'],
    queryFn: api.listMovies,
  })
}

export function useSeats(movieID: string) {
  return useQuery({
    queryKey: ['seats', movieID],
    queryFn: () => api.listSeats(movieID),
    refetchInterval: 5_000,
  })
}

interface HoldVariables {
  movieID: string
  seatID: string
}

export function useHoldSeat() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ movieID, seatID }: HoldVariables) =>
      api.holdSeat(movieID, seatID),
    onSuccess: (res) => {
      void qc.invalidateQueries({ queryKey: ['seats', res.movie_id] })
    },
    onError: (_err, vars) => {
      void qc.invalidateQueries({ queryKey: ['seats', vars.movieID] })
    },
  })
}

interface CheckoutVariables {
  movieID: string
  sessionIDs: string[]
  successURL: string
  cancelURL: string
}

export function useCreateCheckout() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ movieID, sessionIDs, successURL, cancelURL }: CheckoutVariables) =>
      api
        .createCheckout({
          session_ids: sessionIDs,
          success_url: successURL,
          cancel_url: cancelURL,
        })
        .then((res) => ({ res, movieID })),
    onSuccess: ({ movieID }) => {
      void qc.invalidateQueries({ queryKey: ['seats', movieID] })
    },
    onError: (_err, vars) => {
      void qc.invalidateQueries({ queryKey: ['seats', vars.movieID] })
    },
  })
}

interface ReleaseVariables {
  movieID: string
  sessionID: string
}

export function useReleaseSession() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ movieID, sessionID }: ReleaseVariables) =>
      api.releaseSession(sessionID).then(() => movieID),
    onSuccess: (movieID) => {
      void qc.invalidateQueries({ queryKey: ['seats', movieID] })
    },
  })
}

interface AdminConfirmVariables {
  movieID: string
  sessionIDs: string[]
}

// useAdminConfirm confirms held seats at the counter without a card payment
// (staff booking on behalf of a walk-in customer).
export function useAdminConfirm() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ movieID, sessionIDs }: AdminConfirmVariables) =>
      api.adminConfirmSeats(sessionIDs).then((confirmed) => ({ confirmed, movieID })),
    onSuccess: ({ movieID }) => {
      void qc.invalidateQueries({ queryKey: ['seats', movieID] })
    },
    onError: (_err, vars) => {
      void qc.invalidateQueries({ queryKey: ['seats', vars.movieID] })
    },
  })
}

interface AdminCancelVariables {
  movieID: string
  sessionID: string
}

// useAdminCancel voids a confirmed booking so the seat is available again.
export function useAdminCancel() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ movieID, sessionID }: AdminCancelVariables) =>
      api.adminCancelSession(sessionID).then(() => movieID),
    onSuccess: (movieID) => {
      void qc.invalidateQueries({ queryKey: ['seats', movieID] })
    },
    onError: (_err, vars) => {
      void qc.invalidateQueries({ queryKey: ['seats', vars.movieID] })
    },
  })
}
