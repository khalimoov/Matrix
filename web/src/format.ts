export function formatBytes(n: number): string {
  const units = ["Б", "КБ", "МБ", "ГБ", "ТБ"];
  let i = 0;
  while (n >= 1024 && i < units.length - 1) {
    n /= 1024;
    i++;
  }
  return `${n.toFixed(n < 10 && i > 0 ? 1 : 0)} ${units[i]}`;
}

export function formatTime(iso: string | Date): string {
  return new Date(iso).toLocaleString("ru-RU");
}

export function pad(n: number): string {
  return String(n).padStart(2, "0");
}

// Date in the browser's time zone as YYYY-MM-DD for <input type="date">.
export function dateInputValue(d: Date): string {
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

export function errorMessage(e: unknown): string {
  return e instanceof Error ? e.message : String(e);
}
