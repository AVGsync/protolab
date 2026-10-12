import { describe, expect, it } from 'vitest';
import { describeAnomalies, sessionAnomalies } from './anomalies';
import type { SessionFlags } from './types';

const clean: SessionFlags = {
  handshake: true,
  closed: true,
  gaps: 0,
  retransmissions: 0,
  outOfOrder: 0,
  overlapConflicts: 0,
  checksumWarnings: 0,
};

describe('sessionAnomalies', () => {
  it('is empty for a clean session', () => {
    expect(sessionAnomalies(clean, 'handshake')).toEqual([]);
  });

  it('orders faults before warnings and notes', () => {
    const list = sessionAnomalies({ ...clean, gaps: 2, retransmissions: 1, handshake: false, checksumWarnings: 3 }, 'inferred');
    expect(list.map((a) => a.tone)).toEqual(['danger', 'warn', 'info', 'muted', 'muted']);
    expect(list[0].text).toBe('2 пропуска');
  });

  it('describes anomalies as one plain sentence', () => {
    const list = sessionAnomalies({ ...clean, retransmissions: 1, outOfOrder: 1 }, 'handshake');
    expect(describeAnomalies(list)).toBe('1 ретрансмиссия, 1 сегмент вне порядка');
  });
});
