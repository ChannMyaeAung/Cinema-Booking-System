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
