import { createContext, useContext } from "react";
import type { SessionResponse } from "./api/api";

export interface ActiveHold {
  movieID: string;
  seatID: string;
  sessionID: string;
  expiresAt: number;
}

export interface SessionContextValue {
  userID: string;
  activeHolds: ActiveHold[];
  holdSeat: (movieID: string, seatID: string) => Promise<ActiveHold>;
  confirmSeat: (hold: ActiveHold) => Promise<SessionResponse>;
  releaseHold: (hold: ActiveHold) => Promise<void>;
  discardHold: (hold: ActiveHold) => void;
}

export const SessionContext = createContext<SessionContextValue | null>(null);

export function useSession(): SessionContextValue {
  const ctx = useContext(SessionContext);
  if (!ctx) throw new Error("useSession must be used within a SessionProvider");
  return ctx;
}
