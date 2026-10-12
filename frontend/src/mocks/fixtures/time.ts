/** RFC 3339 timestamp `offsetUs` microseconds after `baseIso`, with microsecond precision. */
export function isoAt(baseIso: string, offsetUs: number): string {
  const totalUs = Date.parse(baseIso) * 1000 + Math.round(offsetUs);
  const ms = Math.floor(totalUs / 1000);
  const micro = totalUs - ms * 1000;
  return `${new Date(ms).toISOString().slice(0, 23)}${String(micro).padStart(3, '0')}Z`;
}

export const SECOND = 1_000_000;
export const MS = 1_000;
