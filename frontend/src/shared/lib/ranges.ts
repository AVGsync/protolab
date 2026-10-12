/** Half-open byte range [offset, offset + length). */
export interface ByteRange {
  offset: number;
  length: number;
}

export const rangeEnd = (r: ByteRange): number => r.offset + r.length;

export function containsOffset(r: ByteRange, offset: number): boolean {
  return offset >= r.offset && offset < rangeEnd(r);
}

export function intersects(a: ByteRange, b: ByteRange): boolean {
  return a.offset < rangeEnd(b) && b.offset < rangeEnd(a);
}

/** Range between two offsets, both inclusive, in any order. */
export function rangeBetween(a: number, b: number): ByteRange {
  const start = Math.min(a, b);
  return { offset: start, length: Math.abs(a - b) + 1 };
}

/** Byte mask of `length` where every byte covered by `ranges` is 1. */
export function buildMask(length: number, ranges: readonly ByteRange[]): Uint8Array {
  const mask = new Uint8Array(length);
  for (const r of ranges) {
    const start = Math.max(0, r.offset);
    const end = Math.min(length, rangeEnd(r));
    if (end > start) mask.fill(1, start, end);
  }
  return mask;
}

export function maskHasAny(mask: Uint8Array, r: ByteRange): boolean {
  const end = Math.min(mask.length, rangeEnd(r));
  for (let i = Math.max(0, r.offset); i < end; i++) if (mask[i]) return true;
  return false;
}

/** URL form: "<offset>:<length>". */
export function serializeRange(r: ByteRange): string {
  return `${r.offset}:${r.length}`;
}

export function parseRange(value: string | null | undefined): ByteRange | null {
  if (!value) return null;
  const m = /^(\d+):(\d+)$/.exec(value);
  if (!m) return null;
  const range = { offset: Number(m[1]), length: Number(m[2]) };
  return range.length > 0 ? range : null;
}

export function formatOffset(offset: number): string {
  return `0x${offset.toString(16).toUpperCase().padStart(4, '0')}`;
}

export function formatRange(r: ByteRange): string {
  if (r.length === 1) return `${formatOffset(r.offset)} (1 Б)`;
  return `${formatOffset(r.offset)}–${formatOffset(rangeEnd(r) - 1)} (${r.length} Б)`;
}
