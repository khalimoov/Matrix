import { useState, type FormEvent } from "react";
import { api } from "../api";
import { errorMessage } from "../format";
import { Modal } from "./Modal";

export function PasswordDialog({ onClose }: { onClose: () => void }) {
  const [oldPw, setOldPw] = useState("");
  const [newPw, setNewPw] = useState("");
  const [repeat, setRepeat] = useState("");
  const [error, setError] = useState("");
  const [done, setDone] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (newPw !== repeat) return setError("Пароли не совпадают");
    try {
      await api.changePassword(oldPw, newPw);
      setDone(true);
    } catch (err) {
      setError(errorMessage(err));
    }
  }

  return (
    <Modal title="Смена пароля" onClose={onClose}>
      {done ? (
        <>
          <p>Пароль изменён. Войдите снова с новым паролем.</p>
          <div className="actions">
            <button className="primary" onClick={() => location.reload()}>Войти</button>
          </div>
        </>
      ) : (
        <form onSubmit={submit} className="form">
          <label>Текущий пароль<input type="password" value={oldPw} onChange={(e) => setOldPw(e.target.value)} autoFocus /></label>
          <label>Новый пароль (от 8 символов)<input type="password" value={newPw} onChange={(e) => setNewPw(e.target.value)} /></label>
          <label>Повторите пароль<input type="password" value={repeat} onChange={(e) => setRepeat(e.target.value)} /></label>
          {error && <div className="error">{error}</div>}
          <div className="actions">
            <button type="button" onClick={onClose}>Отмена</button>
            <button className="primary" type="submit">Сохранить</button>
          </div>
        </form>
      )}
    </Modal>
  );
}
