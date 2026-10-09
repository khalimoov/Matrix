import { useEffect, useMemo, useRef, useState } from "react";
import { api, playbackURL, type Camera, type Recording } from "../api";
import { useSession } from "../session";
import { CameraTree } from "../components/CameraTree";
import { dateInputValue, errorMessage, pad } from "../format";

const DAY = 24 * 3600;
const MAX_CHUNK = 3600; // longest piece requested from the server at once
const SPEEDS = [0.5, 1, 2, 4, 8];
const CLIP_LENGTHS = [1, 5, 15, 30, 60]; // minutes

interface Span {
  start: number; // unix seconds
  end: number;
}

export function PlaybackPage() {
  const { cameras } = useSession();
  const [camera, setCamera] = useState<Camera | null>(null);
  const [day, setDay] = useState(dateInputValue(new Date()));
  const [spans, setSpans] = useState<Span[]>([]);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [src, setSrc] = useState<{ url: string; start: number } | null>(null);
  const [cursor, setCursor] = useState<number | null>(null); // unix seconds of the frame on screen
  const [speed, setSpeed] = useState(1);
  const [clipMin, setClipMin] = useState(5);
  const videoRef = useRef<HTMLVideoElement>(null);

  const dayStart = useMemo(() => new Date(`${day}T00:00:00`).getTime() / 1000, [day]);

  useEffect(() => {
    if (!camera) return;
    setLoading(true);
    setError("");
    setSrc(null);
    setCursor(null);
    api
      .recordings(camera.id, new Date(dayStart * 1000), new Date((dayStart + DAY) * 1000))
      .then((r) => setSpans(r.recordings.map(toSpan)))
      .catch((e) => {
        setSpans([]);
        setError(errorMessage(e));
      })
      .finally(() => setLoading(false));
  }, [camera, dayStart]);

  useEffect(() => {
    if (videoRef.current) videoRef.current.playbackRate = speed;
  }, [speed, src]);

  // Play from time t. Outside a recording, play the nearest recorded moment:
  // short recordings are only a pixel or two wide on a 24-hour scale.
  const seek = (t: number, forward = false) => {
    if (!camera || spans.length === 0) return;
    const candidates = forward ? spans.filter((s) => s.end > t) : spans;
    if (candidates.length === 0) return;
    const dist = (s: Span) => (t < s.start ? s.start - t : t > s.end ? t - s.end : 0);
    const span = candidates.reduce((a, b) => (dist(b) < dist(a) ? b : a));
    const start = Math.min(Math.max(t, span.start), Math.max(span.start, span.end - 5));
    const duration = Math.min(span.end - start, MAX_CHUNK);
    setSrc({ url: playbackURL(camera.main_path, new Date(start * 1000), duration), start });
    setCursor(start);
  };

  // Continue with the next piece when one finishes.
  const onEnded = () => {
    if (src && videoRef.current) seek(src.start + videoRef.current.duration + 0.5, true);
  };

  const onTimelineClick = (e: React.MouseEvent<HTMLDivElement>) => {
    const r = e.currentTarget.getBoundingClientRect();
    seek(dayStart + ((e.clientX - r.left) / r.width) * DAY);
  };

  const step = (sec: number) => cursor !== null && seek(cursor + sec);

  return (
    <div className="live">
      <aside className="sidebar">
        <CameraTree cameras={cameras.filter((c) => c.record || c.enabled)} selected={camera?.id} onPick={setCamera} />
      </aside>
      <section className="playback">
        <div className="wall-toolbar">
          <strong>{camera ? camera.name : "Выберите камеру"}</strong>
          <input type="date" value={day} max={dateInputValue(new Date())} onChange={(e) => e.target.value && setDay(e.target.value)} />
          <span className="spacer" />
          {cursor !== null && <span className="mono">{formatClock(cursor)}</span>}
        </div>

        <div className="pb-video">
          {src ? (
            <video
              ref={videoRef}
              key={src.url}
              src={src.url}
              autoPlay
              controls
              muted
              onTimeUpdate={(e) => setCursor(src.start + e.currentTarget.currentTime)}
              onEnded={onEnded}
              onError={() => setError("Не удалось воспроизвести запись")}
            />
          ) : (
            <div className="player-overlay">
              {!camera ? "Выберите камеру слева" : loading ? <span className="spinner" /> : spans.length ? "Нажмите на шкалу времени" : "За этот день записей нет"}
            </div>
          )}
        </div>

        <div className="timeline" onClick={onTimelineClick} title="Нажмите, чтобы воспроизвести с этого момента">
          {spans.map((s, i) => (
            <div
              key={i}
              className="tl-span"
              style={{ left: `${((s.start - dayStart) / DAY) * 100}%`, width: `${Math.max(((s.end - s.start) / DAY) * 100, 0.1)}%` }}
            />
          ))}
          {cursor !== null && <div className="tl-cursor" style={{ left: `${((cursor - dayStart) / DAY) * 100}%` }} />}
          {Array.from({ length: 25 }, (_, h) => (
            <div key={h} className="tl-tick" style={{ left: `${(h / 24) * 100}%` }}>
              {h % 2 === 0 && h < 24 && <span>{pad(h)}:00</span>}
            </div>
          ))}
        </div>

        <div className="pb-controls">
          <button onClick={() => step(-30)} disabled={cursor === null}>« 30 с</button>
          <button onClick={() => step(30)} disabled={cursor === null}>30 с »</button>
          <div className="seg">
            {SPEEDS.map((s) => (
              <button key={s} className={speed === s ? "active" : ""} onClick={() => setSpeed(s)}>{s}×</button>
            ))}
          </div>
          <span className="spacer" />
          <label className="inline">
            Фрагмент
            <select value={clipMin} onChange={(e) => setClipMin(Number(e.target.value))}>
              {CLIP_LENGTHS.map((m) => <option key={m} value={m}>{m} мин</option>)}
            </select>
          </label>
          <a
            className={`button ${cursor === null || !camera ? "disabled" : ""}`}
            href={camera && cursor !== null ? playbackURL(camera.main_path, new Date(cursor * 1000), clipMin * 60, "mp4") : undefined}
            download={camera && cursor !== null ? `${camera.name}_${formatClock(cursor).replace(/[: ]/g, "-")}.mp4` : undefined}
          >
            Скачать MP4
          </a>
        </div>
        {error && <div className="error">{error}</div>}
      </section>
    </div>
  );
}

function toSpan(r: Recording): Span {
  const start = new Date(r.start).getTime() / 1000;
  return { start, end: start + r.duration };
}

function formatClock(t: number): string {
  const d = new Date(t * 1000);
  return `${d.toLocaleDateString("ru-RU")} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
}
