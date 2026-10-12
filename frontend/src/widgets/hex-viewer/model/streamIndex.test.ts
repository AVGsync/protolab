import { describe, expect, it } from 'vitest';
import { buildGapLabels, buildStreamIndex, HIT, HIT_END, HIT_START, SEGMENT_FLAGGED, SEGMENT_START } from './streamIndex';

describe('buildStreamIndex', () => {
  it('marks segment starts and flags the unusual ones', () => {
    const index = buildStreamIndex(
      10,
      [
        { offset: 0, length: 5, packetNumber: 1, timestamp: 't', flags: [] },
        { offset: 5, length: 5, packetNumber: 2, timestamp: 't', flags: ['out_of_order'] },
      ],
      [],
    );
    expect(index[0]).toBe(SEGMENT_START);
    expect(index[5]).toBe(SEGMENT_START | SEGMENT_FLAGGED);
    expect(index[1]).toBe(0);
  });

  it('marks hit ranges with their first and last byte, clipped to the stream', () => {
    const index = buildStreamIndex(8, [], [{ offset: 2, length: 3 }, { offset: 7, length: 5 }]);
    expect(index[2]).toBe(HIT | HIT_START);
    expect(index[3]).toBe(HIT);
    expect(index[4]).toBe(HIT | HIT_END);
    expect(index[7]).toBe(HIT | HIT_START | HIT_END);
  });
});

describe('buildGapLabels', () => {
  it('labels a gap once, in the row where it starts', () => {
    const labels = buildGapLabels([{ offset: 22, length: 12, reason: 'lost' }]);
    expect([...labels.keys()]).toEqual([1]);
    expect(labels.get(1)).toEqual([{ at: 6, cells: 10, text: 'потеря 12 Б' }]);
  });
});
