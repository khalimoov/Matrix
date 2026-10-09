import { useCallback, useEffect, useState, type FormEvent } from "react";
import { api, type Role, type User } from "../api";
import { useSession } from "../session";
import { Modal } from "../components/Modal";
import { errorMessage, formatTime } from "../format";

const ROLES: { value: Role; label: string; hint: string }[] = [
  { value: "admin", label: "Администратор", hint: "всё, включая устройства и пользователей" },
  { value: "operator", label: "Оператор", hint: "просмотр, архив, PTZ, события" },
  { value: "viewer", label: "Наблюдатель", hint: "только живой просмотр" },
];
const roleLabel = (r: Role) => ROLES.find((x) => x.value === r)?.label ?? r;

export function UsersPage() {
  const { user: me } = useSession();
  const [users, setUsers] = useState<User[]>([]);
  const [edit, setEdit] = useState<User | "new" | null>(null);
  const [error, setError] = useState("");

  const load = useCallback(() => api.users().then(setUsers, (e) => setError(errorMessage(e))), []);
  useEffect(() => {
    load();
  }, [load]);

  const remove = async (u: User) => {
    if (!confirm(`Удалить пользователя «${u.username}»?`)) return;
    try {
      await api.deleteUser(u.id);
      load();
    } catch (e) {
      setError(errorMessage(e));
    }
  };

  return (
    <div className="page">
      <div className="page-head">
        <h1>Пользователи</h1>
        <span className="spacer" />
        <button className="primary" onClick={() => setEdit("new")}>+ Добавить пользователя</button>
      </div>
      {error && <div className="error">{error}</div>}
      <table className="table">
        <thead><tr><th>Имя</th><th>Роль</th><th>Создан</th><th /></tr></thead>
        <tbody>
          {users.map((u) => (
            <tr key={u.id}>
              <td><strong>{u.username}</strong>{u.id === me.id && <span className="muted"> (вы)</span>}</td>
              <td>{roleLabel(u.role)}</td>
              <td className="small">{formatTime(u.created_at)}</td>
              <td className="row-actions">
                <button onClick={() => setEdit(u)}>Изменить</button>
                {u.id !== me.id && <button className="danger" onClick={() => remove(u)}>Удалить</button>}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      {edit && (
        <UserDialog
          user={edit === "new" ? null : edit}
          onClose={() => setEdit(null)}
          onSaved={() => {
            setEdit(null);
            load();
          }}
        />
      )}
    </div>
  );
}

function UserDialog({ user, onClose, onSaved }: { user: User | null; onClose: () => void; onSaved: () => void }) {
  const [username, setUsername] = useState(user?.username ?? "");
  const [password, setPassword] = useState("");
  const [role, setRole] = useState<Role>(user?.role ?? "operator");
  const [error, setError] = useState("");

  async function submit(e: FormEvent) {
    e.preventDefault();
    try {
      if (user) await api.updateUser(user.id, role, password);
      else await api.createUser(username, password, role);
      onSaved();
    } catch (err) {
      setError(errorMessage(err));
    }
  }

  return (
    <Modal title={user ? `Пользователь ${user.username}` : "Новый пользователь"} onClose={onClose}>
      <form onSubmit={submit} className="form">
        {!user && <label>Имя пользователя<input value={username} onChange={(e) => setUsername(e.target.value)} required autoFocus /></label>}
        <label>
          {user ? "Новый пароль (оставьте пустым, чтобы не менять)" : "Пароль (от 8 символов)"}
          <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} required={!user} autoComplete="new-password" />
        </label>
        <fieldset className="roles">
          <legend>Роль</legend>
          {ROLES.map((r) => (
            <label key={r.value} className="check">
              <input type="radio" name="role" checked={role === r.value} onChange={() => setRole(r.value)} />
              <span><strong>{r.label}</strong> <span className="muted">— {r.hint}</span></span>
            </label>
          ))}
        </fieldset>
        {error && <div className="error">{error}</div>}
        <div className="actions">
          <button type="button" onClick={onClose}>Отмена</button>
          <button className="primary">Сохранить</button>
        </div>
      </form>
    </Modal>
  );
}
