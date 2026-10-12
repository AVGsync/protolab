import { describe, expect, it } from 'vitest';
import { fieldRange, findFieldAt, findMessageAt, sortByOffset } from './lookup';
import type { Message, ParsedField } from './types';

const field = (name: string, offset: number, length: number, children?: ParsedField[]): ParsedField => ({
  path: name,
  name,
  offset,
  length,
  type: children ? 'array' : 'u8',
  value: null,
  display: '',
  status: 'ok',
  children,
});

const message = (index: number, offset: number, length: number, fields: ParsedField[] = []): Message => ({
  sessionId: 's',
  direction: 'c2s',
  index,
  offset,
  length,
  timestamp: '2026-01-01T00:00:00Z',
  packetNumbers: [],
  ruleId: null,
  status: 'ok',
  issues: [],
  fields,
  unknown: [],
  data: '',
});

describe('findMessageAt', () => {
  const list = sortByOffset([message(2, 20, 5), message(0, 0, 10), message(1, 10, 10)]);

  it('sorts by stream offset and finds the message covering an offset', () => {
    expect(list.map((m) => m.index)).toEqual([0, 1, 2]);
    expect(findMessageAt(list, 0)?.index).toBe(0);
    expect(findMessageAt(list, 9)?.index).toBe(0);
    expect(findMessageAt(list, 10)?.index).toBe(1);
    expect(findMessageAt(list, 24)?.index).toBe(2);
  });

  it('returns nothing outside messages', () => {
    expect(findMessageAt(list, 25)).toBeUndefined();
    expect(findMessageAt([message(0, 5, 5)], 2)).toBeUndefined();
  });
});

describe('findFieldAt', () => {
  const m = message(0, 100, 12, [
    field('head', 0, 2),
    field('items', 2, 8, [field('[0]', 2, 4, [field('a', 2, 1), field('b', 3, 3)]), field('[1]', 6, 4)]),
  ]);

  it('descends to the innermost field', () => {
    expect(findFieldAt(m, 0)?.name).toBe('head');
    expect(findFieldAt(m, 2)?.name).toBe('a');
    expect(findFieldAt(m, 4)?.name).toBe('b');
    expect(findFieldAt(m, 7)?.name).toBe('[1]');
  });

  it('is undefined for bytes outside every field', () => {
    expect(findFieldAt(m, 11)).toBeUndefined();
  });

  it('maps a field to a stream range', () => {
    expect(fieldRange(m, m.fields[1])).toEqual({ offset: 102, length: 8 });
  });
});
