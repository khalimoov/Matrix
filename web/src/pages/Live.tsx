import { useEffect, useMemo, useRef, useState } from "react";
import { roleAtLeast, type Camera } from "../api";
import { useSession } from "../session";
import { CameraTree } from "../components/CameraTree";
import { LivePlayer } from "../components/LivePlayer";
import { PTZPanel } from "../components/PTZPanel";

const LAYOUTS = [1, 4, 9, 16, 25];
const MAX_CELLS = 25;
const STORAGE_KEY = "matrix.live.v1";

interface Cell {
  camera: number | null;
  hd: boolean; // play the main stream instead of the sub stream
}

interface WallState {
  layout: number;
  cells: Cell[];
}

function loadWall(): WallState {
  const empty: WallState = { layout: 4, cells: Array.from({ length: MAX_CELLS }, () => ({ camera: null, hd: false })) };
  try {
    const saved = JSON.parse(localStorage.getItem(STORAGE_KEY) ?? "null") as WallState | null;
    if (saved && LAYOUTS.includes(saved.layout) && Array.isArray(saved.cells)) {
      return { layout: saved.layout, cells: empty.cells.map((c, i) => saved.cells[i] ?? c) };
    }
  } catch {
    /* storage unavailable */
  }
  return empty;
}

export function LivePage() {
  const { cameras, user } = useSession();
  const [wall, setWall] = useState<WallState>(loadWall);
  const [selected, setSelected] = useState(0);
  const [maximized, setMaximized] = useState<number | null>(null);
  const [ptzCell, setPtzCell] = useState<number | null>(null);
  const gridRef = useRef<HTMLDivElement>(null);
  const canPTZ = roleAtLeast(user.role, "operator");

  useEffect(() => {
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(wall));
    } catch {
      /* storage unavailable */
    }
  }, [wall]);

  const byId = useMemo(() => new Map(cameras.map((c) => [c.id, c])), [cameras]);

  const setCell = (i: number, patch: Partial<Cell>) =>
    setWall((w) => ({ ...w, cells: w.cells.map((c, j) => (j === i ? { ...c, ...patch } : c)) }));

  // Put a camera in the selected cell and move the selection to the next one.
  const place = (cam: Camera) => {
    setCell(selected, { camera: cam.id, hd: false });
    setSelected((s) => (s + 1) % wall.layout);
  };

  const drop = (i: number, e: React.DragEvent) => {
    e.preventDefault();
    const camId = Number(e.dataTransfer.getData("text/camera"));
    const from = e.dataTransfer.getData("text/cell");
    if (from !== "") {
      // Swap two cells.
      const j = Number(from);
      setWall((w) => {
        const cells = [...w.cells];
        [cells[i], cells[j]] = [cells[j], cells[i]];
        return { ...w, cells };
      });
    } else if (camId) {
      setCell(i, { camera: camId, hd: false });
    }
    setSelected(i);
  };

  const fillAll = () => {
    const cams = cameras.filter((c) => c.enabled).slice(0, wall.layout);
    setWall((w) => ({ ...w, cells: w.cells.map((_c, i) => (i < cams.length ? { camera: cams[i].id, hd: false } : { camera: null, hd: false })) }));
  };

  const clearAll = () => setWall((w) => ({ ...w, cells: w.cells.map(() => ({ camera: null, hd: false })) }));

  const visible = maximized !== null ? [maximized] : Array.from({ length: wall.layout }, (_, i) => i);
  const cols = Math.sqrt(maximized !== null ? 1 : wall.layout);
  const ptzCamera = ptzCell !== null ? byId.get(wall.cells[ptzCell]?.camera ?? -1) : undefined;

  return (
    <div className="live">
      <aside className="sidebar">
        <CameraTree cameras={cameras} onPick={place} draggable />
        {ptzCamera && (
          <div className="sidebar-panel">
            <div className="panel-title">
              PTZ: {ptzCamera.name}
              <button className="icon" onClick={() => setPtzCell(null)}>✕</button>
            </div>
            <PTZPanel camera={ptzCamera} />
          </div>
        )}
      </aside>
      <section className="wall">
        <div className="wall-toolbar">
          <div className="seg">
            {LAYOUTS.map((n) => (
              <button
                key={n}
                className={wall.layout === n && maximized === null ? "active" : ""}
                onClick={() => {
                  setMaximized(null);
                  setWall((w) => ({ ...w, layout: n }));
                  setSelected((s) => Math.min(s, n - 1));
                }}
                title={`${n} ${n === 1 ? "окно" : "окон"}`}
              >
                <LayoutIcon n={n} />
              </button>
            ))}
          </div>
          <button onClick={fillAll}>Заполнить</button>
          <button onClick={clearAll}>Очистить</button>
          <span className="spacer" />
          <span className="muted small">Нажмите камеру в списке или перетащите её в окно. Двойной клик — развернуть.</span>
          <button onClick={() => gridRef.current?.requestFullscreen()}>На весь экран</button>
        </div>
        <div className="grid" ref={gridRef} style={{ gridTemplateColumns: `repeat(${cols}, 1fr)`, gridTemplateRows: `repeat(${cols}, 1fr)` }}>
          {visible.map((i) => {
            const cell = wall.cells[i];
            const cam = cell.camera !== null ? byId.get(cell.camera) : undefined;
            return (
              <div
                key={i}
                className={`cell ${selected === i ? "selected" : ""}`}
                onClick={() => setSelected(i)}
                onDoubleClick={() => setMaximized((m) => (m === null ? i : null))}
                onDragOver={(e) => e.preventDefault()}
                onDrop={(e) => drop(i, e)}
              >
                {cam ? (
                  <>
                    {cam.enabled ? (
                      <LivePlayer key={`${cam.id}-${cell.hd}`} path={cell.hd ? cam.main_path : cam.live_path} />
                    ) : (
                      <div className="player-overlay">Камера отключена</div>
                    )}
                    <div className="cell-bar" draggable onDragStart={(e) => e.dataTransfer.setData("text/cell", String(i))}>
                      <span className={`dot ${cam.status?.online ? "ok" : "bad"}`} />
                      <span className="cell-name">{cam.name}</span>
                      {cam.record && <span className="rec" title="Идёт запись">●</span>}
                      <span className="spacer" />
                      {cam.live_path !== cam.main_path && (
                        <button className={`chip ${cell.hd ? "active" : ""}`} onClick={() => setCell(i, { hd: !cell.hd })} title="Основной поток">
                          HD
                        </button>
                      )}
                      {cam.ptz && canPTZ && (
                        <button className={`chip ${ptzCell === i ? "active" : ""}`} onClick={() => setPtzCell(ptzCell === i ? null : i)}>
                          PTZ
                        </button>
                      )}
                      <button className="chip" onClick={() => setCell(i, { camera: null })} title="Закрыть">✕</button>
                    </div>
                  </>
                ) : (
                  <div className="cell-empty">{i + 1}</div>
                )}
              </div>
            );
          })}
        </div>
      </section>
    </div>
  );
}

function LayoutIcon({ n }: { n: number }) {
  const k = Math.sqrt(n);
  const s = 14 / k;
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" aria-hidden>
      {Array.from({ length: n }, (_, i) => (
        <rect key={i} x={1 + (i % k) * s} y={1 + Math.floor(i / k) * s} width={s - 1} height={s - 1} rx="0.5" fill="currentColor" />
      ))}
    </svg>
  );
}
