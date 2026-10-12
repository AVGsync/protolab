import { describe, expect, it } from 'vitest';
import { base64ToBytes, bytesToBase64, decodeAt, encodeInt, findValue, parseHex, readInt, toHex } from './bytes';
import { buildMask } from './ranges';

describe('hex helpers', () => {
  it('parses common hex notations', () => {
    expect(toHex(parseHex('A5 01')!)).toBe('A5 01');
    expect(toHex(parseHex('0xa5,0x01')!)).toBe('A5 01');
    expect(toHex(parseHex('a5:01:ff')!)).toBe('A5 01 FF');
    expect(parseHex('A5 0')).toBeNull();
    expect(parseHex('zz')).toBeNull();
  });

  it('round-trips base64', () => {
    const bytes = Uint8Array.from([0, 1, 2, 250, 255]);
    expect(base64ToBytes(bytesToBase64(bytes))).toEqual(bytes);
  });
});

describe('integers', () => {
  it('encodes and decodes signed little-endian values', () => {
    const bytes = encodeInt(-2, 'i16le')!;
    expect(toHex(bytes)).toBe('FE FF');
    expect(readInt(bytes, 0, 'i16le')).toBe(-2);
    expect(readInt(bytes, 0, 'u16le')).toBe(0xfffe);
  });

  it('rejects values that do not fit', () => {
    expect(encodeInt(300, 'u8')).toBeNull();
    expect(encodeInt(-1, 'u16le')).toBeNull();
    expect(encodeInt(1.5, 'u32le')).toBeNull();
  });

  it('returns null when reading past the end', () => {
    expect(readInt(Uint8Array.from([1]), 0, 'u16le')).toBeNull();
  });

  it('decodes all interpretations at an offset', () => {
    const rows = decodeAt(Uint8Array.from([0x15, 0x00, 0x00, 0x00]), 0);
    expect(rows.find((r) => r.label === 'u16le')?.value).toBe('21');
    expect(rows.find((r) => r.label === 'u32le')?.value).toBe('21');
  });
});

describe('findValue', () => {
  // A5 02 01 03 00 | 01 15 00 | checksum
  const frame = Uint8Array.from([0xa5, 0x02, 0x01, 0x03, 0x00, 0x01, 0x15, 0x00, 0xd9]);

  it('merges encodings that produce identical bytes', () => {
    const hits = findValue(frame, 21, ['u8', 'u16le', 'i16le']);
    expect(hits).toEqual([
      { offset: 6, length: 1, encodings: ['u8'] },
      { offset: 6, length: 2, encodings: ['u16le', 'i16le'] },
    ]);
  });

  it('never matches inside unknown bytes', () => {
    const zeros = new Uint8Array(8);
    const mask = buildMask(8, [{ offset: 0, length: 6 }]);
    const hits = findValue(zeros, 0, ['u16le'], mask);
    expect(hits.map((h) => h.offset)).toEqual([6]);
  });
});
