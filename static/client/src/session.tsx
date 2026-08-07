import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { useUser } from "@clerk/react";
import * as api from "./api/api";
import { SessionContext, type ActiveHold } from "./session-context";

const HOLDS_KEY = "cinema.holds";

function readJSON<T>(key: string, fallback: T): T {
  try {
    const raw = localStorage.getItem(key);
    return raw ? (JSON.parse(raw) as T) : fallback;
  } catch {
    return fallback;
  }
}

export function SessionProvider({ children }: { children: ReactNode }) {
  // Identity comes from Clerk: the authenticated user id, or "" when
  // signed out. Booking (holding a seat) requires a signed-in user.
  const { user } = useUser();
  const userID = user?.id ?? "";

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
    if (!userID) return;
    const all = readJSON<ActiveHold[]>(HOLDS_KEY, []);
    const now = Date.now();
    for (const hold of all) {
      if (hold.expiresAt <= now) {
        void api.releaseSession(hold.sessionID, userID).catch(() => {});
      }
    }
  }, [userID]);

  // When the identity changes (sign-out, or a different account in the
  // same browser), the previous user's holds no longer belong to us:
  // release them server-side (best effort) and clear them locally.
  // Skipped while prev is empty so the initial Clerk load
  // ('' -> user.id) doesn't wipe persisted holds on page refresh.
  const prevUserID = useRef(userID);
  useEffect(() => {
    const prev = prevUserID.current;
    prevUserID.current = userID;
    if (!prev || prev === userID) return;
    for (const hold of holds) {
      void api.releaseSession(hold.sessionID, prev).catch(() => {});
    }
    setHolds([]);
  }, [userID, holds]);

  const addHold = useCallback((hold: ActiveHold) => {
    setHolds((prev) => [...prev.filter((h) => h.seatID !== hold.seatID), hold]);
  }, []);

  const removeHold = useCallback((sessionID: string) => {
    setHolds((prev) => prev.filter((h) => h.sessionID !== sessionID));
  }, []);

  const value = useMemo(
    () => ({ userID, activeHolds: holds, addHold, removeHold }),
    [userID, holds, addHold, removeHold],
  );

  return (
    <SessionContext.Provider value={value}>{children}</SessionContext.Provider>
  );
}
