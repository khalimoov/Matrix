import { useEffect, useRef, useState } from "react";
import { api, type Camera, type Preset } from "../api";
import { errorMessage } from "../format";

const SPEED = 0.5;

// PTZPanel moves the camera while a button is held and stops on release.
export function PTZPanel({ camera }: { camera: Camera }) {
  const [presets, setPresets] = useState<Preset[]>([]);
  const [error, setError] = useState("");
  const moving = useRef(false);

  useEffect(() => {
    api.ptzPresets(camera.id).then(setPresets, (e) => setError(errorMessage(e)));
  }, [camera.id]);

  const start = (pan: number, tilt: number, zoom: number) => {
    moving.current = true;
    api.ptzMove(camera.id, pan, tilt, zoom).catch((e) => setError(errorMessage(e)));
  };
  const stop = () => {
    if (!moving.current) return;
    moving.current = false;
    api.ptzStop(camera.id).catch(() => {});
  };

  const btn = (label: string, pan: number, tilt: number, zoom = 0, title = "") => (
    <button
      className="ptz-btn"
      title={title}
      onPointerDown={(e) => {
        e.currentTarget.setPointerCapture(e.pointerId);
        start(pan, tilt, zoom);
      }}
      onPointerUp={stop}
      onPointerCancel={stop}
    >
      {label}
    </button>
  );

  return (
    <div className="ptz">
      <div className="ptz-pad">
        {btn("↖", -SPEED, SPEED)}
        {btn("▲", 0, SPEED)}
        {btn("↗", SPEED, SPEED)}
        {btn("◀", -SPEED, 0)}
        <span />
        {btn("▶", SPEED, 0)}
        {btn("↙", -SPEED, -SPEED)}
        {btn("▼", 0, -SPEED)}
        {btn("↘", SPEED, -SPEED)}
      </div>
      <div className="ptz-zoom">
        {btn("＋", 0, 0, SPEED, "Приблизить")}
        {btn("－", 0, 0, -SPEED, "Отдалить")}
      </div>
      {presets.length > 0 && (
        <select defaultValue="" onChange={(e) => e.target.value && api.ptzGoto(camera.id, e.target.value).catch((err) => setError(errorMessage(err)))}>
          <option value="" disabled>Пресеты…</option>
          {presets.map((p) => (
            <option key={p.token} value={p.token}>{p.name || p.token}</option>
          ))}
        </select>
      )}
      {error && <div className="error small">{error}</div>}
    </div>
  );
}
