import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from "react";
import { api, type Camera, type MatrixEvent, type SystemInfo, type User } from "./api";

// Session holds what every page needs: the signed-in user, the camera list
// (refreshed every few seconds for live status) and the stream of new events.
interface Session {
  user: User;
  cameras: Camera[];
  system: SystemInfo | null;
  reloadCameras: () => Promise<void>;
  subscribe: (fn: (e: MatrixEvent) => void) => () => void;
  logout: () => Promise<void>;
}

const SessionContext = createContext<Session | null>(null);

export function useSession(): Session {
  const s = useContext(SessionContext);
  if (!s) throw new Error("useSession outside SessionProvider");
  return s;
}

export function SessionProvider({ user, onLogout, children }: { user: User; onLogout: () => void; children: ReactNode }) {
  const [cameras, setCameras] = useState<Camera[]>([]);
  const [system, setSystem] = useState<SystemInfo | null>(null);
  const listeners = useRef(new Set<(e: MatrixEvent) => void>());

  const reloadCameras = useCallback(async () => {
    try {
      const [cams, sys] = await Promise.all([api.cameras(), api.system()]);
      setCameras(cams);
      setSystem(sys);
    } catch {
      /* shown as stale data; the next poll retries */
    }
  }, []);

  useEffect(() => {
    reloadCameras();
    const t = setInterval(reloadCameras, 5000);
    return () => clearInterval(t);
  }, [reloadCameras]);

  useEffect(() => {
    const es = new EventSource("/api/events/stream");
    es.onmessage = (msg) => {
      const e: MatrixEvent = JSON.parse(msg.data);
      listeners.current.forEach((fn) => fn(e));
      if (e.type.startsWith("camera_")) reloadCameras();
    };
    return () => es.close();
  }, [reloadCameras]);

  const subscribe = useCallback((fn: (e: MatrixEvent) => void) => {
    listeners.current.add(fn);
    return () => {
      listeners.current.delete(fn);
    };
  }, []);

  const logout = useCallback(async () => {
    await api.logout().catch(() => {});
    onLogout();
  }, [onLogout]);

  return (
    <SessionContext.Provider value={{ user, cameras, system, reloadCameras, subscribe, logout }}>
      {children}
    </SessionContext.Provider>
  );
}
