import { useState, type FormEvent } from "react";
import { api, type Camera, type CameraInput, type OnvifProbe, type Vendor } from "../api";
import { errorMessage } from "../format";
import { Modal } from "./Modal";

const VENDORS: { value: Vendor; label: string }[] = [
  { value: "dahua", label: "Dahua (и OEM: Imou, Amcrest…)" },
  { value: "hikvision", label: "Hikvision (и OEM: HiWatch…)" },
  { value: "onvif", label: "ONVIF (любой производитель)" },
  { value: "generic", label: "RTSP-адрес вручную" },
];

export function emptyCamera(): CameraInput {
  return {
    name: "",
    group: "",
    vendor: "dahua",
    host: "",
    rtsp_port: 554,
    username: "admin",
    password: "",
    channel: 1,
    main_url: "",
    sub_url: "",
    onvif_url: "",
    onvif_profile: "",
    record: true,
    retention_days: 7,
    enabled: true,
  };
}

export function cameraToInput(c: Camera): CameraInput {
  const { id: _id, created_at: _c, has_password: _h, status: _s, live_path: _l, main_path: _m, ptz: _p, sync_error: _e, ...rest } = c;
  return { ...rest, password: null };
}

// Strip credentials from a URL returned by ONVIF: they are stored separately.
function withoutCredentials(raw: string): string {
  try {
    const u = new URL(raw);
    u.username = "";
    u.password = "";
    return u.toString();
  } catch {
    return raw;
  }
}

export function CameraForm({
  initial,
  editing,
  hasPassword,
  groups,
  onClose,
  onSaved,
}: {
  initial: CameraInput;
  editing?: number;
  hasPassword?: boolean;
  groups: string[];
  onClose: () => void;
  onSaved: (c: Camera) => void;
}) {
  const [c, setC] = useState<CameraInput>(initial);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [probe, setProbe] = useState<OnvifProbe | null>(null);
  const [probing, setProbing] = useState(false);

  const set = <K extends keyof CameraInput>(k: K, v: CameraInput[K]) => setC((p) => ({ ...p, [k]: v }));
  const byVendorURL = c.vendor === "dahua" || c.vendor === "hikvision";

  async function runProbe() {
    const url = c.onvif_url || (c.host ? `http://${c.host}/onvif/device_service` : "");
    if (!url) return setError("Укажите адрес ONVIF или IP камеры");
    setProbing(true);
    setError("");
    try {
      const res = await api.probe(url, c.username, c.password ?? "", editing);
      setProbe(res);
      const ptz = res.profiles.find((p) => p.has_ptz);
      setC((p) => ({
        ...p,
        onvif_url: url,
        onvif_profile: ptz?.token ?? "",
        name: p.name || `${res.device.manufacturer} ${res.device.model}`.trim(),
        ...(p.vendor === "onvif" || p.vendor === "generic"
          ? {
              vendor: "onvif" as Vendor,
              main_url: res.profiles[0] ? withoutCredentials(res.profiles[0].stream_uri) : "",
              sub_url: res.profiles[1] ? withoutCredentials(res.profiles[1].stream_uri) : "",
            }
          : {}),
      }));
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setProbing(false);
    }
  }

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      const saved = editing ? await api.updateCamera(editing, c) : await api.createCamera(c);
      onSaved(saved);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal title={editing ? "Изменить камеру" : "Добавить камеру"} onClose={onClose} wide>
      <form onSubmit={submit} className="form two-col">
        <label>Название<input value={c.name} onChange={(e) => set("name", e.target.value)} required autoFocus /></label>
        <label>
          Группа
          <input value={c.group} onChange={(e) => set("group", e.target.value)} list="groups" placeholder="Например: Склад" />
          <datalist id="groups">{groups.map((g) => <option key={g} value={g} />)}</datalist>
        </label>
        <label className="span2">
          Тип подключения
          <select value={c.vendor} onChange={(e) => set("vendor", e.target.value as Vendor)}>
            {VENDORS.map((v) => <option key={v.value} value={v.value}>{v.label}</option>)}
          </select>
        </label>

        {byVendorURL && (
          <>
            <label>IP-адрес или имя<input value={c.host} onChange={(e) => set("host", e.target.value)} required placeholder="192.168.1.108" /></label>
            <div className="row2">
              <label>RTSP-порт<input type="number" value={c.rtsp_port} onChange={(e) => set("rtsp_port", Number(e.target.value))} /></label>
              <label title="Номер канала: 1 для камеры, 1…N для регистратора">Канал<input type="number" min={1} value={c.channel} onChange={(e) => set("channel", Number(e.target.value))} /></label>
            </div>
          </>
        )}

        <label>Логин<input value={c.username} onChange={(e) => set("username", e.target.value)} autoComplete="off" /></label>
        <label>
          Пароль
          <input
            type="password"
            value={c.password ?? ""}
            placeholder={editing && hasPassword ? "не менять" : ""}
            onChange={(e) => set("password", e.target.value)}
            autoComplete="new-password"
          />
        </label>

        {!byVendorURL && (
          <>
            <label className="span2">Основной поток (RTSP)<input value={c.main_url} onChange={(e) => set("main_url", e.target.value)} placeholder="rtsp://192.168.1.10:554/stream1" required /></label>
            <label className="span2">Дополнительный поток (необязательно)<input value={c.sub_url} onChange={(e) => set("sub_url", e.target.value)} placeholder="rtsp://192.168.1.10:554/stream2" /></label>
          </>
        )}

        <label className="span2">
          Адрес ONVIF (для PTZ и автоматической настройки)
          <div className="input-row">
            <input value={c.onvif_url} onChange={(e) => set("onvif_url", e.target.value)} placeholder="http://192.168.1.108/onvif/device_service" />
            <button type="button" onClick={runProbe} disabled={probing}>{probing ? "Проверка…" : "Проверить"}</button>
          </div>
        </label>
        {probe && (
          <div className="span2 probe">
            <strong>{probe.device.manufacturer} {probe.device.model}</strong> · прошивка {probe.device.firmware_version}
            <ul>
              {probe.profiles.map((p) => (
                <li key={p.token}>
                  {p.name}: {p.encoding} {p.width}×{p.height} {p.has_ptz && "· PTZ"}
                </li>
              ))}
            </ul>
          </div>
        )}

        <label className="check"><input type="checkbox" checked={c.record} onChange={(e) => set("record", e.target.checked)} /> Непрерывная запись</label>
        <label>Хранить записи, дней<input type="number" min={1} max={3650} value={c.retention_days} onChange={(e) => set("retention_days", Number(e.target.value))} /></label>
        <label className="check"><input type="checkbox" checked={c.enabled} onChange={(e) => set("enabled", e.target.checked)} /> Камера включена</label>

        {error && <div className="error span2">{error}</div>}
        <div className="actions span2">
          <button type="button" onClick={onClose}>Отмена</button>
          <button className="primary" disabled={busy}>{busy ? "Сохранение…" : "Сохранить"}</button>
        </div>
      </form>
    </Modal>
  );
}
