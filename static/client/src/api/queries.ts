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
  userID: string
}

export function useHoldSeat() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ movieID, seatID, userID }: HoldVariables) =>
      api.holdSeat(movieID, seatID, userID),
    onSuccess: (res) => {
      void qc.invalidateQueries({ queryKey: ['seats', res.movie_id] })
    },
    onError: (_err, vars) => {
      void qc.invalidateQueries({ queryKey: ['seats', vars.movieID] })
    },
  })
}

interface SessionVariables {
  movieID: string
  sessionID: string
  userID: string
}

export function useConfirmSession() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ movieID, sessionID, userID }: SessionVariables) =>
      api.confirmSession(sessionID, userID).then((res) => ({ res, movieID })),
    onSuccess: ({ res }) => {
      void qc.invalidateQueries({ queryKey: ['seats', res.movie_id] })
    },
  })
}

export function useReleaseSession() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ movieID, sessionID, userID }: SessionVariables) =>
      api.releaseSession(sessionID, userID).then(() => movieID),
    onSuccess: (movieID) => {
      void qc.invalidateQueries({ queryKey: ['seats', movieID] })
    },
  })
}
