// Typed client for the Matrix server API.

export type Role = "admin" | "operator" | "viewer";
export type Vendor = "dahua" | "hikvision" | "onvif" | "generic";

export interface User {
  id: number;
  username: string;
  role: Role;
  created_at: string;
}

export interface StreamStatus {
  online: boolean;
  readers: number;
  inbound_bytes: number;
  codecs: string[] | null;
}

export interface Camera {
  id: number;
  name: string;
  group: string;
  vendor: Vendor;
  host: string;
  rtsp_port: number;
  username: string;
  channel: number;
  main_url: string;
  sub_url: string;
  onvif_url: string;
  onvif_profile: string;
  record: boolean;
  retention_days: number;
  enabled: boolean;
  created_at: string;
  has_password: boolean;
  status: StreamStatus | null;
  live_path: string;
  main_path: string;
  ptz: boolean;
  sync_error?: string;
}

export type CameraInput = Omit<
  Camera,
  "id" | "created_at" | "has_password" | "status" | "live_path" | "main_path" | "ptz" | "sync_error"
> & { password?: string | null };

export interface MatrixEvent {
  id: number;
  time: string;
  type: string;
  camera_id?: number;
  message: string;
}

export interface SystemInfo {
  version: string;
  uptime_seconds: number;
  media_online: boolean;
  cameras_total: number;
  cameras_enabled: number;
  cameras_online: number;
  recording: number;
  viewers: number;
  disk_total_bytes?: number;
  disk_free_bytes?: number;
}

export interface Recording {
  start: string;
  duration: number;
}

export interface Discovered {
  address: string;
  device_url: string;
  name: string;
  hardware: string;
  added: boolean;
}

export interface OnvifProfile {
  token: string;
  name: string;
  encoding: string;
  width: number;
  height: number;
  has_ptz: boolean;
  stream_uri: string;
}

export interface OnvifProbe {
  device: { manufacturer: string; model: string; firmware_version: string; serial_number: string };
  profiles: OnvifProfile[];
}

export interface Preset {
  token: string;
  name: string;
}

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

// Called when the session has expired so the app can show the login page.
let onUnauthorized = () => {};
export function setUnauthorizedHandler(fn: () => void) {
  onUnauthorized = fn;
}

async function request<T>(method: string, url: string, body?: unknown): Promise<T> {
  const res = await fetch(url, {
    method,
    credentials: "same-origin",
    headers: body !== undefined ? { "Content-Type": "application/json" } : undefined,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  if (res.status === 401 && url !== "/api/auth/login") onUnauthorized();
  if (!res.ok) {
    let msg = res.statusText;
    try {
      msg = (await res.json()).error ?? msg;
    } catch {
      /* not JSON */
    }
    throw new ApiError(res.status, msg);
  }
  if (res.status === 204) return undefined as T;
  return res.json();
}

export const api = {
  login: (username: string, password: string) => request<User>("POST", "/api/auth/login", { username, password }),
  logout: () => request<void>("POST", "/api/auth/logout"),
  me: () => request<User>("GET", "/api/auth/me"),
  changePassword: (old_password: string, new_password: string) =>
    request<void>("POST", "/api/auth/password", { old_password, new_password }),

  system: () => request<SystemInfo>("GET", "/api/system"),

  cameras: () => request<Camera[]>("GET", "/api/cameras"),
  createCamera: (c: CameraInput) => request<Camera>("POST", "/api/cameras", c),
  updateCamera: (id: number, c: CameraInput) => request<Camera>("PUT", `/api/cameras/${id}`, c),
  deleteCamera: (id: number) => request<void>("DELETE", `/api/cameras/${id}`),
  recordings: (id: number, start: Date, end: Date) =>
    request<{ path: string; recordings: Recording[] }>(
      "GET",
      `/api/cameras/${id}/recordings?start=${encodeURIComponent(rfc3339(start))}&end=${encodeURIComponent(rfc3339(end))}`,
    ),

  ptzMove: (id: number, pan: number, tilt: number, zoom: number) =>
    request<void>("POST", `/api/cameras/${id}/ptz/move`, { pan, tilt, zoom }),
  ptzStop: (id: number) => request<void>("POST", `/api/cameras/${id}/ptz/stop`),
  ptzPresets: (id: number) => request<Preset[]>("GET", `/api/cameras/${id}/ptz/presets`),
  ptzGoto: (id: number, token: string) =>
    request<void>("POST", `/api/cameras/${id}/ptz/presets/${encodeURIComponent(token)}`),

  discover: () => request<Discovered[]>("POST", "/api/onvif/discover"),
  probe: (device_url: string, username: string, password: string, camera_id?: number) =>
    request<OnvifProbe>("POST", "/api/onvif/probe", { device_url, username, password, camera_id }),

  events: (params: { before?: number; camera_id?: number; types?: string[]; limit?: number }) => {
    const q = new URLSearchParams();
    if (params.before) q.set("before", String(params.before));
    if (params.camera_id) q.set("camera_id", String(params.camera_id));
    if (params.types?.length) q.set("types", params.types.join(","));
    if (params.limit) q.set("limit", String(params.limit));
    return request<MatrixEvent[]>("GET", `/api/events?${q}`);
  },

  users: () => request<User[]>("GET", "/api/users"),
  createUser: (username: string, password: string, role: Role) =>
    request<User>("POST", "/api/users", { username, password, role }),
  updateUser: (id: number, role: Role, password: string) =>
    request<User>("PUT", `/api/users/${id}`, { role, password }),
  deleteUser: (id: number) => request<void>("DELETE", `/api/users/${id}`),
};

export function rfc3339(d: Date): string {
  return d.toISOString().replace(/\.\d{3}Z$/, "Z");
}

export function playbackURL(path: string, start: Date, durationSec: number, format: "fmp4" | "mp4" = "fmp4") {
  const q = new URLSearchParams({
    path,
    start: rfc3339(start),
    duration: String(Math.max(1, Math.round(durationSec))),
    format,
  });
  return `/media/playback/get?${q}`;
}

export function roleAtLeast(role: Role, min: Role): boolean {
  const rank: Record<Role, number> = { viewer: 1, operator: 2, admin: 3 };
  return rank[role] >= rank[min];
}
