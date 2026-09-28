export type ImportStatus = "running" | "success" | "failed";

export interface ImportRun {
  id: number;
  period_year: number;
  status: ImportStatus;
  started_at: string;
  finished_at: string | null;
  details: Record<string, { rows: number }>;
  error: string | null;
}

export interface Source {
  name: string;
  title: string;
  target_table: string;
}

export interface Health {
  postgres: string;
  oracle: string;
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const resp = await fetch(path, {
    ...init,
    headers: { "Content-Type": "application/json", ...init?.headers },
  });
  if (!resp.ok) {
    const body = await resp.json().catch(() => null);
    const detail = typeof body?.detail === "string" ? body.detail : resp.statusText;
    throw new Error(detail);
  }
  return resp.json() as Promise<T>;
}

export const api = {
  health: () => request<Health>("/api/health"),
  sources: () => request<Source[]>("/api/imports/sources"),
  runs: () => request<ImportRun[]>("/api/imports"),
  startImport: (year: number) =>
    request<ImportRun>("/api/imports", { method: "POST", body: JSON.stringify({ year }) }),
};
