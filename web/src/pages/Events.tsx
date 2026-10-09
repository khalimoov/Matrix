import { useEffect, useMemo, useState } from "react";
import { api, type MatrixEvent } from "../api";
import { useSession } from "../session";
import { errorMessage, formatTime } from "../format";

export const eventNames: Record<string, string> = {
  camera_online: "Камера в сети",
  camera_offline: "Потеря сигнала",
  camera_added: "Камера добавлена",
  camera_changed: "Камера изменена",
  camera_deleted: "Камера удалена",
  login: "Вход",
  login_failed: "Неудачный вход",
  user_changed: "Пользователи",
  system: "Система",
};

const alarmTypes = new Set(["camera_offline", "login_failed", "system"]);

const FILTERS: { label: string; types: string[] }[] = [
  { label: "Все", types: [] },
  { label: "Тревоги", types: ["camera_offline", "login_failed", "system"] },
  { label: "Камеры", types: ["camera_online", "camera_offline", "camera_added", "camera_changed", "camera_deleted"] },
  { label: "Пользователи", types: ["login", "login_failed", "user_changed"] },
];

const PAGE = 100;

export function EventsPage() {
  const { cameras, subscribe } = useSession();
  const [filter, setFilter] = useState(0);
  const [cameraId, setCameraId] = useState(0);
  const [events, setEvents] = useState<MatrixEvent[]>([]);
  const [more, setMore] = useState(false);
  const [error, setError] = useState("");

  const names = useMemo(() => new Map(cameras.map((c) => [c.id, c.name])), [cameras]);
  const types = FILTERS[filter].types;

  useEffect(() => {
    setEvents([]);
    api
      .events({ types, camera_id: cameraId, limit: PAGE })
      .then((list) => {
        setEvents(list);
        setMore(list.length === PAGE);
      })
      .catch((e) => setError(errorMessage(e)));
  }, [filter, cameraId]);

  // Prepend events as they happen.
  useEffect(
    () =>
      subscribe((e) => {
        if (types.length && !types.includes(e.type)) return;
        if (cameraId && e.camera_id !== cameraId) return;
        setEvents((list) => (list.some((x) => x.id === e.id) ? list : [e, ...list]));
      }),
    [subscribe, types, cameraId],
  );

  const loadMore = async () => {
    const last = events[events.length - 1];
    if (!last) return;
    const list = await api.events({ types, camera_id: cameraId, limit: PAGE, before: last.id });
    setEvents((e) => [...e, ...list]);
    setMore(list.length === PAGE);
  };

  return (
    <div className="page">
      <div className="page-head">
        <h1>Журнал событий</h1>
        <div className="seg">
          {FILTERS.map((f, i) => (
            <button key={f.label} className={filter === i ? "active" : ""} onClick={() => setFilter(i)}>{f.label}</button>
          ))}
        </div>
        <select value={cameraId} onChange={(e) => setCameraId(Number(e.target.value))}>
          <option value={0}>Все камеры</option>
          {cameras.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
        </select>
      </div>
      {error && <div className="error">{error}</div>}
      <table className="table">
        <thead><tr><th>Время</th><th>Событие</th><th>Камера</th><th>Описание</th></tr></thead>
        <tbody>
          {events.length === 0 && <tr><td colSpan={4} className="muted center">Событий нет</td></tr>}
          {events.map((e) => (
            <tr key={e.id} className={alarmTypes.has(e.type) ? "alarm" : ""}>
              <td className="mono small nowrap">{formatTime(e.time)}</td>
              <td className="nowrap">{eventNames[e.type] ?? e.type}</td>
              <td>{e.camera_id ? names.get(e.camera_id) ?? `#${e.camera_id}` : ""}</td>
              <td>{e.message}</td>
            </tr>
          ))}
        </tbody>
      </table>
      {more && <div className="actions left"><button onClick={loadMore}>Показать ещё</button></div>}
    </div>
  );
}
