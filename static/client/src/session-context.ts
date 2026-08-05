import { createContext, useContext } from "react";
import type { HoldInfo } from "./api/api";

export interface ActiveHold {
  movieID: string;
  seatID: string;
  sessionID: string;
  expiresAt: number;
}

export interface SessionContextValue {
  userID: string;
  activeHolds: ActiveHold[];
  addHold: (hold: ActiveHold) => void;
  removeHold: (sessionID: string) => void;
}

export const SessionContext = createContext<SessionContextValue | null>(null);

export function useSession(): SessionContextValue {
  const ctx = useContext(SessionContext);
  if (!ctx) throw new Error("useSession must be used within a SessionProvider");
  return ctx;
}

export function holdFromResponse(res: HoldInfo): ActiveHold {
  return {
    movieID: res.movie_id,
    seatID: res.seat_id,
    sessionID: res.session_id,
    expiresAt: new Date(res.expires_at).getTime(),
  };
}
