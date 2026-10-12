export type IntEncoding =
  | 'u8'
  | 'i8'
  | 'u16le'
  | 'u16be'
  | 'i16le'
  | 'i16be'
  | 'u32le'
  | 'u32be'
  | 'i32le'
  | 'i32be';

export const INT_ENCODINGS: readonly IntEncoding[] = [
  'u8',
  'i8',
  'u16le',
  'u16be',
  'i16le',
  'i16be',
  'u32le',
  'u32be',
  'i32le',
  'i32be',
];

export const ENCODING_SIZE: Record<IntEncoding, 1 | 2 | 4> = {
  u8: 1,
  i8: 1,
  u16le: 2,
  u16be: 2,
  i16le: 2,
  i16be: 2,
  u32le: 4,
  u32be: 4,
  i32le: 4,
  i32be: 4,
};

const HEX_TABLE = Array.from({ length: 256 }, (_, i) => i.toString(16).toUpperCase().padStart(2, '0'));

export const hexByte = (b: number): string => HEX_TABLE[b & 0xff];

export function toHex(bytes: Uint8Array, separator = ' '): string {
  const out = new Array<string>(bytes.length);
  for (let i = 0; i < bytes.length; i++) out[i] = HEX_TABLE[bytes[i]];
  return out.join(separator);
}

/** Hex dump where unknown bytes (mask === 1) are shown as "??" — never as values. */
export function toHexMasked(bytes: Uint8Array, unknownMask?: Uint8Array, separator = ' '): string {
  const out = new Array<string>(bytes.length);
  for (let i = 0; i < bytes.length; i++) out[i] = unknownMask?.[i] ? '??' : HEX_TABLE[bytes[i]];
  return out.join(separator);
}

/** Accepts "A5 01", "a501", "0xA5, 0x01", "A5:01". Returns null on invalid input. */
export function parseHex(input: string): Uint8Array | null {
  const cleaned = input.replace(/0x/gi, '').replace(/[\s,:;-]/g, '');
  if (cleaned.length === 0 || cleaned.length % 2 !== 0 || /[^0-9a-f]/i.test(cleaned)) return null;
  const out = new Uint8Array(cleaned.length / 2);
  for (let i = 0; i < out.length; i++) out[i] = parseInt(cleaned.slice(i * 2, i * 2 + 2), 16);
  return out;
}

export function base64ToBytes(b64: string): Uint8Array {
  const bin = atob(b64);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

export function bytesToBase64(bytes: Uint8Array): string {
  let bin = '';
  const chunk = 0x8000;
  for (let i = 0; i < bytes.length; i += chunk) {
    bin += String.fromCharCode(...bytes.subarray(i, i + chunk));
  }
  return btoa(bin);
}

export const isPrintable = (b: number): boolean => b >= 0x20 && b < 0x7f;

export function toAscii(bytes: Uint8Array, placeholder = '.', unknownMask?: Uint8Array): string {
  let s = '';
  for (let i = 0; i < bytes.length; i++) {
    const b = bytes[i];
    s += unknownMask?.[i] ? '·' : isPrintable(b) ? String.fromCharCode(b) : placeholder;
  }
  return s;
}

function view(bytes: Uint8Array): DataView {
  return new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
}

/** Reads an integer; null when the value does not fit into the buffer. */
export function readInt(bytes: Uint8Array, offset: number, enc: IntEncoding): number | null {
  if (offset < 0 || offset + ENCODING_SIZE[enc] > bytes.length) return null;
  const dv = view(bytes);
  switch (enc) {
    case 'u8':
      return dv.getUint8(offset);
    case 'i8':
      return dv.getInt8(offset);
    case 'u16le':
      return dv.getUint16(offset, true);
    case 'u16be':
      return dv.getUint16(offset, false);
    case 'i16le':
      return dv.getInt16(offset, true);
    case 'i16be':
      return dv.getInt16(offset, false);
    case 'u32le':
      return dv.getUint32(offset, true);
    case 'u32be':
      return dv.getUint32(offset, false);
    case 'i32le':
      return dv.getInt32(offset, true);
    case 'i32be':
      return dv.getInt32(offset, false);
  }
}

const RANGES: Record<IntEncoding, [number, number]> = {
  u8: [0, 0xff],
  i8: [-0x80, 0x7f],
  u16le: [0, 0xffff],
  u16be: [0, 0xffff],
  i16le: [-0x8000, 0x7fff],
  i16be: [-0x8000, 0x7fff],
  u32le: [0, 0xffffffff],
  u32be: [0, 0xffffffff],
  i32le: [-0x80000000, 0x7fffffff],
  i32be: [-0x80000000, 0x7fffffff],
};

/** Encodes an integer; null when it is not representable in the encoding. */
export function encodeInt(value: number, enc: IntEncoding): Uint8Array | null {
  const [min, max] = RANGES[enc];
  if (!Number.isInteger(value) || value < min || value > max) return null;
  const out = new Uint8Array(ENCODING_SIZE[enc]);
  const dv = view(out);
  switch (enc) {
    case 'u8':
      dv.setUint8(0, value);
      break;
    case 'i8':
      dv.setInt8(0, value);
      break;
    case 'u16le':
    case 'u16be':
      dv.setUint16(0, value, enc === 'u16le');
      break;
    case 'i16le':
    case 'i16be':
      dv.setInt16(0, value, enc === 'i16le');
      break;
    case 'u32le':
    case 'u32be':
      dv.setUint32(0, value, enc === 'u32le');
      break;
    case 'i32le':
    case 'i32be':
      dv.setInt32(0, value, enc === 'i32le');
      break;
  }
  return out;
}

export interface ValueHit {
  offset: number;
  length: number;
  /** All encodings producing exactly these bytes for the searched value. */
  encodings: IntEncoding[];
}

/** Naive substring search; good enough for streams up to a few MiB. */
export function indexOfAll(haystack: Uint8Array, needle: Uint8Array): number[] {
  const hits: number[] = [];
  if (needle.length === 0 || needle.length > haystack.length) return hits;
  outer: for (let i = 0; i <= haystack.length - needle.length; i++) {
    for (let j = 0; j < needle.length; j++) {
      if (haystack[i + j] !== needle[j]) continue outer;
    }
    hits.push(i);
  }
  return hits;
}

/**
 * Finds a numeric value in all requested encodings. Hits overlapping unknown
 * bytes (`unknownMask[i] === 1`, e.g. capture gaps) are dropped: unknown bytes
 * must never be treated as zeros.
 */
export function findValue(
  haystack: Uint8Array,
  value: number,
  encodings: readonly IntEncoding[],
  unknownMask?: Uint8Array,
): ValueHit[] {
  const byKey = new Map<string, ValueHit>();
  const patterns = new Map<string, { bytes: Uint8Array; encodings: IntEncoding[] }>();

  for (const enc of encodings) {
    const bytes = encodeInt(value, enc);
    if (!bytes) continue;
    const key = toHex(bytes, '');
    const existing = patterns.get(key);
    if (existing) existing.encodings.push(enc);
    else patterns.set(key, { bytes, encodings: [enc] });
  }

  for (const { bytes, encodings: encs } of patterns.values()) {
    for (const offset of indexOfAll(haystack, bytes)) {
      if (unknownMask && unknownMask.subarray(offset, offset + bytes.length).some((b) => b === 1)) continue;
      const key = `${offset}:${bytes.length}`;
      const hit = byKey.get(key);
      if (hit) hit.encodings.push(...encs);
      else byKey.set(key, { offset, length: bytes.length, encodings: [...encs] });
    }
  }

  return [...byKey.values()].sort((a, b) => a.offset - b.offset || a.length - b.length);
}

export interface DecodedValue {
  label: string;
  value: string;
}

/** All common numeric interpretations of the bytes starting at `offset`. */
export function decodeAt(bytes: Uint8Array, offset: number, limit = bytes.length): DecodedValue[] {
  const slice = bytes.subarray(0, Math.min(bytes.length, limit));
  const rows: DecodedValue[] = [];
  for (const enc of INT_ENCODINGS) {
    const v = readInt(slice, offset, enc);
    if (v !== null) rows.push({ label: enc, value: v.toLocaleString('ru-RU') });
  }
  if (offset + 4 <= slice.length) {
    const dv = view(slice);
    rows.push({ label: 'f32le', value: formatFloat(dv.getFloat32(offset, true)) });
    rows.push({ label: 'f32be', value: formatFloat(dv.getFloat32(offset, false)) });
  }
  return rows;
}

function formatFloat(v: number): string {
  if (!Number.isFinite(v)) return String(v);
  if (v !== 0 && (Math.abs(v) < 1e-4 || Math.abs(v) >= 1e9)) return v.toExponential(4);
  return Number(v.toPrecision(7)).toString();
}
