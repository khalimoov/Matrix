import { useState, type FormEvent } from "react";
import { api, type User } from "../api";
import { errorMessage } from "../format";

export function LoginPage({ onLogin }: { onLogin: (u: User) => void }) {
  const [username, setUsername] = useState("admin");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      onLogin(await api.login(username, password));
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="login">
      <form onSubmit={submit} className="login-card">
        <div className="brand big">
          <img src="/favicon.svg" alt="" />
          <span>Matrix</span>
        </div>
        <p className="muted">Система управления видеонаблюдением</p>
        <label>Пользователь<input value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="username" /></label>
        <label>
          Пароль
          <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" autoFocus />
        </label>
        {error && <div className="error">{error}</div>}
        <button className="primary" disabled={busy}>{busy ? "Вход…" : "Войти"}</button>
      </form>
    </div>
  );
}
