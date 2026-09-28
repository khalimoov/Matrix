import { useCallback, useEffect, useState } from "react";
import { api, type Health, type ImportRun, type Source } from "./api";

const STATUS_LABEL: Record<ImportRun["status"], string> = {
  running: "Выполняется",
  success: "Успешно",
  failed: "Ошибка",
};

// Импорт выполняется после Нового года — по умолчанию предлагаем прошлый год.
const DEFAULT_YEAR = new Date().getFullYear() - 1;

function formatDate(value: string | null) {
  return value ? new Date(value).toLocaleString("ru-RU") : "—";
}

export default function App() {
  const [health, setHealth] = useState<Health | null>(null);
  const [sources, setSources] = useState<Source[]>([]);
  const [runs, setRuns] = useState<ImportRun[]>([]);
  const [year, setYear] = useState(DEFAULT_YEAR);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const running = runs.some((r) => r.status === "running");

  const loadRuns = useCallback(() => {
    api.runs().then(setRuns, (e: Error) => setError(e.message));
  }, []);

  useEffect(() => {
    api.health().then(setHealth, () => setHealth(null));
    api.sources().then(setSources, (e: Error) => setError(e.message));
    loadRuns();
  }, [loadRuns]);

  // Пока идёт импорт — обновляем статус каждые 3 секунды.
  useEffect(() => {
    if (!running) return;
    const id = setInterval(loadRuns, 3000);
    return () => clearInterval(id);
  }, [running, loadRuns]);

  async function startImport() {
    const hasRun = runs.some((r) => r.period_year === year && r.status === "success");
    const question = hasRun
      ? `Данные за ${year} год уже загружены. Загрузить заново и заменить их?`
      : `Запустить загрузку данных из Oracle за ${year} год?`;
    if (!window.confirm(question)) return;

    setSubmitting(true);
    setError(null);
    try {
      await api.startImport(year);
      loadRuns();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <main>
      <h1>Matrix</h1>

      <section className="card">
        <h2>Подключения</h2>
        <p>
          PostgreSQL: <StatusBadge value={health?.postgres} />
          <br />
          Oracle: <StatusBadge value={health?.oracle} />
        </p>
      </section>

      <section className="card">
        <h2>Импорт из Oracle</h2>
        <p className="muted">
          Загружаются источники: {sources.map((s) => s.title).join(", ") || "—"}
        </p>
        <div className="row">
          <label>
            Год{" "}
            <input
              type="number"
              min={1990}
              max={2100}
              value={year}
              onChange={(e) => setYear(Number(e.target.value))}
            />
          </label>
          <button onClick={startImport} disabled={running || submitting}>
            {running ? "Импорт выполняется…" : "Запустить импорт"}
          </button>
        </div>
        {error && <p className="error">{error}</p>}
      </section>

      <section className="card">
        <h2>История загрузок</h2>
        {runs.length === 0 ? (
          <p className="muted">Загрузок ещё не было.</p>
        ) : (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>#</th>
                  <th>Год</th>
                  <th>Статус</th>
                  <th>Начало</th>
                  <th>Окончание</th>
                  <th>Строк</th>
                </tr>
              </thead>
              <tbody>
                {runs.map((r) => (
                  <tr key={r.id}>
                    <td>{r.id}</td>
                    <td>{r.period_year}</td>
                    <td>
                      <span className={`status status-${r.status}`}>{STATUS_LABEL[r.status]}</span>
                      {r.error && <div className="error small">{r.error}</div>}
                    </td>
                    <td>{formatDate(r.started_at)}</td>
                    <td>{formatDate(r.finished_at)}</td>
                    <td>
                      {Object.values(r.details).reduce((sum, d) => sum + d.rows, 0) || "—"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </main>
  );
}

function StatusBadge({ value }: { value: string | undefined }) {
  if (value === undefined) return <span className="muted">проверка…</span>;
  if (value === "ok") return <span className="status status-success">доступна</span>;
  return (
    <span className="status status-failed" title={value}>
      недоступна
    </span>
  );
}
