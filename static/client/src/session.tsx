import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import * as api from "./api/api";
import { SessionContext, type ActiveHold } from "./session-context";

const USER_ID_KEY = "cinema.userId";
const HOLDS_KEY = "cinema.holds";

function readJSON<T>(key: string, fallback: T): T {
  try {
    const raw = localStorage.getItem(key);
    return raw ? (JSON.parse(raw) as T) : fallback;
  } catch {
    return fallback;
  }
}

function newUserID(): string {
  return `u_${crypto.randomUUID().slice(0, 13)}`;
}

export function SessionProvider({ children }: { children: ReactNode }) {
  const [userID] = useState<string>(() => {
    const existing = localStorage.getItem(USER_ID_KEY);
    if (existing) return existing;
    const id = newUserID();
    localStorage.setItem(USER_ID_KEY, id);
    return id;
  });

  const [holds, setHolds] = useState<ActiveHold[]>(() =>
    readJSON<ActiveHold[]>(HOLDS_KEY, []).filter(
      (h) => h.expiresAt > Date.now(),
    ),
  );

  useEffect(() => {
    localStorage.setItem(HOLDS_KEY, JSON.stringify(holds));
  }, [holds]);

  // Release holds that expired while the app was closed.
  useEffect(() => {
    const all = readJSON<ActiveHold[]>(HOLDS_KEY, []);
    const now = Date.now();
    for (const hold of all) {
      if (hold.expiresAt <= now) {
        void api.releaseSession(hold.sessionID, userID).catch(() => {});
      }
    }
  }, [userID]);

  const releaseHold = useCallback(
    async (hold: ActiveHold) => {
      try {
        await api.releaseSession(hold.sessionID, userID);
      } catch {
        // The hold may already be gone server-side; clear it locally either way.
      } finally {
        setHolds((prev) => prev.filter((h) => h.sessionID !== hold.sessionID));
      }
    },
    [userID],
  );

  const holdSeat = useCallback(
    async (movieID: string, seatID: string) => {
      const res = await api.holdSeat(movieID, seatID, userID);
      const hold: ActiveHold = {
        movieID: res.movie_id,
        seatID: res.seat_id,
        sessionID: res.session_id,
        expiresAt: new Date(res.expires_at).getTime(),
      };
      setHolds((prev) => [...prev.filter((h) => h.seatID !== seatID), hold]);
      return hold;
    },
    [userID],
  );

  const confirmSeat = useCallback(
    async (hold: ActiveHold) => {
      const res = await api.confirmSession(hold.sessionID, userID);
      setHolds((prev) => prev.filter((h) => h.sessionID !== hold.sessionID));
      return res;
    },
    [userID],
  );

  const discardHold = useCallback((hold: ActiveHold) => {
    setHolds((prev) => prev.filter((h) => h.sessionID !== hold.sessionID));
  }, []);

  const value = useMemo(
    () => ({
      userID,
      activeHolds: holds,
      holdSeat,
      confirmSeat,
      releaseHold,
      discardHold,
    }),
    [userID, holds, holdSeat, confirmSeat, releaseHold, discardHold],
  );

  return (
    <SessionContext.Provider value={value}>{children}</SessionContext.Provider>
  );
}
