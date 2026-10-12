import { describe, expect, it } from 'vitest';
import { encodeInt } from './bytes';
import { columnStats, findFieldCandidates } from './compare';

function setParamFrame(value: number): Uint8Array {
  const v = encodeInt(value, 'i16le')!;
  return Uint8Array.from([0xa5, 0x02, 0x07, 0x03, 0x00, 0x01, v[0], v[1], 0x00]);
}

describe('columnStats', () => {
  it('classifies constant, variable and partial columns', () => {
    const stats = columnStats([
      { bytes: Uint8Array.from([1, 2, 3]) },
      { bytes: Uint8Array.from([1, 5]) },
    ]);
    expect(stats.map((s) => s.kind)).toEqual(['constant', 'variable', 'partial']);
  });
});

describe('findFieldCandidates', () => {
  it('ranks a 16-bit field above a byte when a value exceeds 255', () => {
    const rows = [21, 23, 18, 300].map((param) => ({ bytes: setParamFrame(param), param }));
    const { candidates, warning } = findFieldCandidates(rows);
    expect(warning).toBeUndefined();
    expect(candidates[0]).toMatchObject({ offset: 6, encoding: 'u16le', scale: 1, matches: 4, total: 4 });
    const u8 = candidates.find((c) => c.offset === 6 && c.encoding === 'u8');
    expect(u8?.counterexamples).toEqual([3]);
  });

  it('explains when parameters do not differ', () => {
    const rows = [21, 21].map((param) => ({ bytes: setParamFrame(param), param }));
    expect(findFieldCandidates(rows).warning).toMatch(/одинаковое/);
  });

  it('finds scaled values', () => {
    const rows = [22.4, 19.8, -5.5].map((param) => ({
      bytes: Uint8Array.from([0xa5, ...encodeInt(Math.round(param * 10), 'i16le')!]),
      param,
    }));
    const { candidates } = findFieldCandidates(rows);
    expect(candidates[0]).toMatchObject({ offset: 1, encoding: 'i16le', scale: 10, matches: 3 });
  });
});
