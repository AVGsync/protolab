const nf = new Intl.NumberFormat('ru-RU');

export const formatNumber = (n: number): string => nf.format(n);

export function formatBytes(n: number): string {
  if (n < 1024) return `${n} Б`;
  const units = ['КиБ', 'МиБ', 'ГиБ'];
  let v = n / 1024;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(v >= 100 ? 0 : 1)} ${units[i]}`;
}

export function formatPercent(ratio: number, digits = 1): string {
  return `${(ratio * 100).toFixed(digits)} %`;
}

export function formatDateTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString('ru-RU', {
    day: '2-digit',
    month: '2-digit',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  });
}

/**
 * Capture timestamp in UTC with sub-millisecond precision taken from the
 * RFC 3339 string itself (Date would truncate to milliseconds).
 */
export function formatTimestamp(iso: string, fractionDigits = 6): string {
  const m = /T(\d{2}:\d{2}:\d{2})(?:\.(\d+))?Z$/.exec(iso);
  if (!m) return iso;
  if (fractionDigits === 0) return m[1];
  const frac = (m[2] ?? '').padEnd(fractionDigits, '0').slice(0, fractionDigits);
  return `${m[1]}.${frac}`;
}

export function diffMs(fromIso: string, toIso: string): number {
  return new Date(toIso).getTime() - new Date(fromIso).getTime();
}

export function formatDuration(ms: number): string {
  if (ms < 1) return `${(ms * 1000).toFixed(0)} мкс`;
  if (ms < 1000) return `${ms.toFixed(ms < 10 ? 2 : 0)} мс`;
  const s = ms / 1000;
  if (s < 60) return `${s.toFixed(s < 10 ? 2 : 1)} с`;
  const m = Math.floor(s / 60);
  return `${m} мин ${Math.round(s - m * 60)} с`;
}

export function shortHash(hash: string, length = 12): string {
  return hash.length > length ? hash.slice(0, length) : hash;
}

/** Russian plural forms: plural(5, ['пакет', 'пакета', 'пакетов']). */
export function plural(n: number, forms: readonly [string, string, string]): string {
  const mod10 = n % 10;
  const mod100 = n % 100;
  if (mod10 === 1 && mod100 !== 11) return forms[0];
  if (mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14)) return forms[1];
  return forms[2];
}

export const countLabel = (n: number, forms: readonly [string, string, string]): string =>
  `${formatNumber(n)} ${plural(n, forms)}`;
