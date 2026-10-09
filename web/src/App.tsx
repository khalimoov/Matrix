import { useCallback, useEffect, useState } from "react";
import { Navigate, NavLink, Route, Routes } from "react-router-dom";
import { api, roleAtLeast, setUnauthorizedHandler, type User } from "./api";
import { SessionProvider, useSession } from "./session";
import { LoginPage } from "./pages/Login";
import { LivePage } from "./pages/Live";
import { PlaybackPage } from "./pages/Playback";
import { DevicesPage } from "./pages/Devices";
import { EventsPage } from "./pages/Events";
import { UsersPage } from "./pages/Users";
import { PasswordDialog } from "./components/PasswordDialog";
import { formatBytes } from "./format";

export function App() {
  const [user, setUser] = useState<User | null | undefined>(undefined);

  useEffect(() => {
    api.me().then(setUser, () => setUser(null));
    setUnauthorizedHandler(() => setUser(null));
  }, []);

  const onLogout = useCallback(() => setUser(null), []);

  if (user === undefined) return <div className="splash"><span className="spinner" /></div>;
  if (user === null) return <LoginPage onLogin={setUser} />;
  return (
    <SessionProvider user={user} onLogout={onLogout}>
      <Shell />
    </SessionProvider>
  );
}

const roleNames = { admin: "Администратор", operator: "Оператор", viewer: "Наблюдатель" };

function Shell() {
  const { user, system, logout } = useSession();
  const [pwOpen, setPwOpen] = useState(false);
  const canOperate = roleAtLeast(user.role, "operator");
  const isAdmin = user.role === "admin";

  return (
    <div className="shell">
      <header className="topbar">
        <div className="brand">
          <img src="/favicon.svg" alt="" />
          <span>Matrix</span>
        </div>
        <nav>
          <NavLink to="/live">Просмотр</NavLink>
          {canOperate && <NavLink to="/playback">Архив</NavLink>}
          {canOperate && <NavLink to="/events">События</NavLink>}
          {isAdmin && <NavLink to="/devices">Устройства</NavLink>}
          {isAdmin && <NavLink to="/users">Пользователи</NavLink>}
        </nav>
        <div className="topbar-status">
          {system && (
            <>
              <span className={`pill ${system.media_online ? "ok" : "bad"}`}>
                {system.media_online ? "Медиасервер" : "Медиасервер недоступен"}
              </span>
              <span className="pill" title="Камеры в сети / всего">
                Камеры {system.cameras_online}/{system.cameras_enabled}
              </span>
              {system.disk_free_bytes !== undefined && (
                <span className="pill" title="Свободно на диске для записи">
                  Диск {formatBytes(system.disk_free_bytes)}
                </span>
              )}
            </>
          )}
        </div>
        <div className="usermenu">
          <span title={roleNames[user.role]}>{user.username}</span>
          <button className="link" onClick={() => setPwOpen(true)}>Пароль</button>
          <button className="link" onClick={logout}>Выйти</button>
        </div>
      </header>
      <main>
        <Routes>
          <Route path="/live" element={<LivePage />} />
          {canOperate && <Route path="/playback" element={<PlaybackPage />} />}
          {canOperate && <Route path="/events" element={<EventsPage />} />}
          {isAdmin && <Route path="/devices" element={<DevicesPage />} />}
          {isAdmin && <Route path="/users" element={<UsersPage />} />}
          <Route path="*" element={<Navigate to="/live" replace />} />
        </Routes>
      </main>
      {pwOpen && <PasswordDialog onClose={() => setPwOpen(false)} />}
    </div>
  );
}
