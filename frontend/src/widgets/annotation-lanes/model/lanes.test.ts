import { describe, expect, it } from 'vitest';
import type { Message, ParsedField } from '@/entities/message';
import { fieldLane, frameLane, rulerTicks, tcpLane } from './lanes';

const message = (offset: number, length: number, fields: ParsedField[] = []): Message => ({
  sessionId: 's',
  direction: 's2c',
  index: 1,
  offset,
  length,
  timestamp: '2026-01-01T00:00:00Z',
  packetNumbers: [],
  ruleId: 'r',
  status: 'ok',
  issues: [],
  fields,
  unknown: [],
  data: '',
});

describe('tcpLane', () => {
  it('cuts segments and gaps to the message bounds', () => {
    const lane = tcpLane(
      message(50, 100),
      [
        { offset: 0, length: 60, packetNumber: 101, timestamp: 't', flags: [] },
        { offset: 60, length: 60, packetNumber: 102, timestamp: 't', flags: ['retransmission'] },
        { offset: 140, length: 60, packetNumber: 103, timestamp: 't', flags: [] },
      ],
      [{ offset: 120, length: 20, reason: 'lost' }],
    );
    expect(lane.map((s) => [s.label, s.offset, s.length])).toEqual([
      ['#101  10 Б', 0, 10],
      ['#102  60 Б', 10, 60],
      ['потеря 20 Б', 70, 20],
      ['#103  10 Б', 90, 10],
    ]);
    expect(lane[2].kind).toBe('gap');
    expect(lane[1].title).toContain('retransmission');
  });
});

describe('frameLane', () => {
  it('splits a length-prefixed frame into header, data and trailer', () => {
    const framing = { kind: 'length_prefixed', lengthField: { offset: 3, size: 2, endian: 'le' }, lengthAdjust: 6 } as const;
    expect(frameLane(framing, 167).map((s) => [s.label, s.offset, s.length])).toEqual([
      ['заголовок 5', 0, 5],
      ['данные 161', 5, 161],
      ['хвост 1', 166, 1],
    ]);
  });

  it('survives messages shorter than the header', () => {
    const framing = { kind: 'length_prefixed', lengthField: { offset: 3, size: 2, endian: 'le' }, lengthAdjust: 6 } as const;
    expect(frameLane(framing, 4).map((s) => s.length)).toEqual([4]);
  });

  it('handles delimiter and fixed framing, and no framing at all', () => {
    expect(frameLane({ kind: 'delimiter', delimiter: '0D0A' }, 10).map((s) => s.length)).toEqual([8, 2]);
    expect(frameLane({ kind: 'fixed', size: 8 }, 8).map((s) => s.label)).toEqual(['кадр 8']);
    expect(frameLane(undefined, 8)).toEqual([]);
  });
});

describe('fieldLane', () => {
  it('turns top-level fields into toned segments and marks violations', () => {
    const base = { value: 0, display: '1', status: 'ok', type: 'u8' } as const;
    const lane = fieldLane(
      message(0, 4, [
        { ...base, path: 'a', name: 'a', offset: 0, length: 1 },
        { ...base, path: 'sum', name: 'sum', offset: 3, length: 1, status: 'violation' },
      ]),
    );
    expect(lane.map((s) => [s.label, s.tone, s.kind])).toEqual([
      ['a', 0, undefined],
      ['sum', 1, 'violation'],
    ]);
  });
});

describe('rulerTicks', () => {
  it('picks a round step and always ends on the last byte', () => {
    const ticks = rulerTicks(167, 900);
    expect(ticks[0]).toBe(0);
    expect(ticks[ticks.length - 1]).toBe(166);
    expect(ticks.slice(0, 3)).toEqual([0, 16, 32]);
  });

  it('never puts two labels closer than the minimum gap (167 B once printed "160166")', () => {
    for (const [length, width] of [[167, 900], [167, 500], [64, 700], [1000, 800]]) {
      const ticks = rulerTicks(length, width);
      const px = width / length;
      for (let i = 1; i < ticks.length; i++) expect((ticks[i] - ticks[i - 1]) * px).toBeGreaterThanOrEqual(48 - 1e-9);
    }
  });

  it('degrades gracefully', () => {
    expect(rulerTicks(1, 500)).toEqual([0]);
    expect(rulerTicks(100, 0)).toEqual([0]);
  });
});
