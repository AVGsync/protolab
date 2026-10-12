import { describe, expect, it } from 'vitest';
import type { Message, ParsedField } from '@/entities/message';
import { ALT, buildRowCells, COVERED, FIELD_END, FIELD_START, VIOLATION } from './row';

const field = (name: string, offset: number, length: number, extra: Partial<ParsedField> = {}): ParsedField => ({
  path: name,
  name,
  offset,
  length,
  type: 'u8',
  value: 0,
  display: '0',
  status: 'ok',
  ...extra,
});

const message = (index: number, offset: number, length: number, fields: ParsedField[], status: Message['status'] = 'ok'): Message => ({
  sessionId: 's',
  direction: 'c2s',
  index,
  offset,
  length,
  timestamp: '2026-01-01T00:00:00Z',
  packetNumbers: [],
  ruleId: 'r',
  status,
  issues: [],
  fields,
  unknown: [],
  data: '',
});

describe('buildRowCells', () => {
  const items = [field('[0]', 2, 2), field('[1]', 4, 2)];
  const m = message(0, 16, 20, [
    field('a', 0, 2),
    field('arr', 2, 4, { type: 'array', children: items }),
    field('sum', 19, 1, { status: 'violation' }),
  ]);

  it('marks field edges, covered bytes and the message start', () => {
    const row = buildRowCells(1, [m]);
    expect(row.starts).toEqual([{ at: 0, index: 0, violated: false }]);
    expect(row.flags[0] & (COVERED | FIELD_START)).toBe(COVERED | FIELD_START);
    expect(row.flags[1] & FIELD_END).toBe(FIELD_END);
    expect(row.tone[0]).toBe(0);
    expect(row.tone[2]).toBe(1);
    expect(row.flags[6] & COVERED).toBe(0);
  });

  it('alternates array elements and keeps their own edges', () => {
    const row = buildRowCells(1, [m]);
    expect(row.flags[2] & ALT).toBe(0);
    expect(row.flags[4] & ALT).toBe(ALT);
    expect(row.flags[3] & FIELD_END).toBe(FIELD_END);
    expect(row.flags[4] & FIELD_START).toBe(FIELD_START);
  });

  it('flags violating leaf fields and reports the message covering each byte', () => {
    const second = buildRowCells(2, [m]);
    expect(second.flags[3] & VIOLATION).toBe(VIOLATION);
    expect(second.message[3]).toBe(0);
    expect(second.message[4]).toBe(-1);
    expect(second.starts).toEqual([]);
  });

  it('lists several messages that start in one row', () => {
    const row = buildRowCells(0, [message(0, 0, 4, []), message(1, 4, 6, []), message(2, 10, 20, [])]);
    expect(row.starts.map((s) => [s.at, s.index])).toEqual([
      [0, 0],
      [4, 1],
      [10, 2],
    ]);
  });
});
