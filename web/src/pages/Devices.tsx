import { useMemo, useState } from "react";
import { api, type Camera, type CameraInput, type Discovered } from "../api";
import { useSession } from "../session";
import { CameraForm, cameraToInput, emptyCamera } from "../components/CameraForm";
import { Modal } from "../components/Modal";
import { errorMessage, formatBytes } from "../format";

const vendorNames: Record<string, string> = { dahua: "Dahua", hikvision: "Hikvision", onvif: "ONVIF", generic: "RTSP" };

export function DevicesPage() {
  const { cameras, reloadCameras } = useSession();
  const [form, setForm] = useState<{ input: CameraInput; id?: number; hasPassword?: boolean } | null>(null);
  const [discoverOpen, setDiscoverOpen] = useState(false);
  const [notice, setNotice] = useState("");

  const groups = useMemo(() => [...new Set(cameras.map((c) => c.group).filter(Boolean))].sort(), [cameras]);

  const remove = async (c: Camera) => {
    if (!confirm(`Удалить камеру «${c.name}»? Записи на диске сохранятся до окончания срока хранения.`)) return;
    try {
      await api.deleteCamera(c.id);
      reloadCameras();
    } catch (e) {
      alert(errorMessage(e));
    }
  };

  const onSaved = (c: Camera) => {
    setForm(null);
    setNotice(c.sync_error ? `Камера сохранена, но медиасервер вернул ошибку: ${c.sync_error}` : "");
    reloadCameras();
  };

  return (
    <div className="page">
      <div className="page-head">
        <h1>Устройства</h1>
        <span className="spacer" />
        <button onClick={() => setDiscoverOpen(true)}>Поиск в сети (ONVIF)</button>
        <button className="primary" onClick={() => setForm({ input: emptyCamera() })}>+ Добавить камеру</button>
      </div>
      {notice && <div className="error">{notice}</div>}
      <table className="table">
        <thead>
          <tr>
            <th>Статус</th>
            <th>Название</th>
            <th>Группа</th>
            <th>Тип</th>
            <th>Адрес</th>
            <th>Кодек</th>
            <th>Запись</th>
            <th>Трафик</th>
            <th />
          </tr>
        </thead>
        <tbody>
          {cameras.length === 0 && (
            <tr><td colSpan={9} className="muted center">Камер пока нет. Добавьте камеру вручную или найдите её в сети.</td></tr>
          )}
          {cameras.map((c) => (
            <tr key={c.id} className={c.enabled ? "" : "dim"}>
              <td>
                <span className={`dot ${!c.enabled ? "off" : c.status?.online ? "ok" : "bad"}`} />{" "}
                {!c.enabled ? "Отключена" : c.status?.online ? "В сети" : "Нет сигнала"}
              </td>
              <td><strong>{c.name}</strong></td>
              <td>{c.group}</td>
              <td>{vendorNames[c.vendor]}{c.ptz && " · PTZ"}</td>
              <td className="mono small">{c.host || hostOf(c.main_url)}{c.channel > 1 && ` / к.${c.channel}`}</td>
              <td>{c.status?.codecs?.join(", ")}</td>
              <td>{c.record ? `${c.retention_days} дн.` : "—"}</td>
              <td className="small">{c.status ? formatBytes(c.status.inbound_bytes) : ""}</td>
              <td className="row-actions">
                <button onClick={() => setForm({ input: cameraToInput(c), id: c.id, hasPassword: c.has_password })}>Изменить</button>
                <button className="danger" onClick={() => remove(c)}>Удалить</button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>

      {form && (
        <CameraForm
          initial={form.input}
          editing={form.id}
          hasPassword={form.hasPassword}
          groups={groups}
          onClose={() => setForm(null)}
          onSaved={onSaved}
        />
      )}
      {discoverOpen && (
        <DiscoverDialog
          onClose={() => setDiscoverOpen(false)}
          onPick={(d) => {
            setDiscoverOpen(false);
            setForm({ input: { ...emptyCamera(), vendor: "onvif", name: d.name, host: d.address, onvif_url: d.device_url } });
          }}
        />
      )}
    </div>
  );
}

function hostOf(url: string): string {
  try {
    return new URL(url).host;
  } catch {
    return url;
  }
}

function DiscoverDialog({ onClose, onPick }: { onClose: () => void; onPick: (d: Discovered) => void }) {
  const [devices, setDevices] = useState<Discovered[] | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const scan = async () => {
    setBusy(true);
    setError("");
    try {
      setDevices(await api.discover());
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal title="Поиск камер в сети" onClose={onClose} wide>
      <p className="muted">
        Поиск ONVIF-устройств в локальной сети сервера (WS-Discovery). На камерах Dahua и Hikvision ONVIF должен быть включён в настройках.
      </p>
      <div className="actions left">
        <button className="primary" onClick={scan} disabled={busy}>{busy ? "Поиск…" : devices ? "Искать снова" : "Начать поиск"}</button>
      </div>
      {error && <div className="error">{error}</div>}
      {devices && (
        <table className="table">
          <thead><tr><th>IP</th><th>Имя</th><th>Модель</th><th /></tr></thead>
          <tbody>
            {devices.length === 0 && <tr><td colSpan={4} className="muted center">Устройства не найдены</td></tr>}
            {devices.map((d) => (
              <tr key={d.device_url}>
                <td className="mono">{d.address}</td>
                <td>{d.name}</td>
                <td>{d.hardware}</td>
                <td className="row-actions">
                  {d.added ? <span className="muted">Уже добавлена</span> : <button onClick={() => onPick(d)}>Добавить</button>}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </Modal>
  );
}
