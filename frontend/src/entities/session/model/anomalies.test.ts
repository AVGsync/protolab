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

  it('tells damaged packets from checksum offloading by the diagnostic code', () => {
    const damaged = sessionAnomalies({ ...clean, checksumWarnings: 1 }, 'handshake', [
      { level: 'warning', code: 'checksum_error', message: '…', count: 1, packetNumbers: [18] },
    ]);
    expect(damaged.map((a) => [a.tone, a.text])).toEqual([['warn', '1 повреждённый пакет']]);
    const offload = sessionAnomalies({ ...clean, checksumWarnings: 6 }, 'handshake', [
      { level: 'info', code: 'checksum_offload', message: '…', count: 6 },
    ]);
    expect(offload.map((a) => a.text)).toEqual(['checksum: 6']);
  });

  it('shows a failed reassembly alone and a stream cut at the limit as a fault', () => {
    const failed = sessionAnomalies({ ...clean, handshake: false }, 'unknown', [{ level: 'error', code: 'reassembly_failed', message: '…' }]);
    expect(failed.map((a) => a.text)).toEqual(['сборка не удалась']);
    const cut = sessionAnomalies({ ...clean, retransmissions: 1 }, 'handshake', [{ level: 'error', code: 'stream_too_large', message: '…' }]);
    expect(cut[0]).toMatchObject({ tone: 'danger', text: 'поток обрезан пределом' });
  });

  it('describes anomalies as one plain sentence', () => {
    const list = sessionAnomalies({ ...clean, retransmissions: 1, outOfOrder: 1 }, 'handshake');
    expect(describeAnomalies(list)).toBe('1 ретрансмиссия, 1 сегмент вне порядка');
  });
});
