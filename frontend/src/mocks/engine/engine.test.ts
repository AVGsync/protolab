import { describe, expect, it } from 'vitest';
import { db, sessionInput } from '../db';
import { SPEC_V2, SPEC_V3 } from '../fixtures/research';
import { parseSession, parseSessionDetailed, validateSpec } from './interpreter';

const run = (id: string) => db.runs.find((r) => r.run.id === id)!;
const sessionByPort = (port: number, captureId: string) =>
  [...db.sessions.values()].find((s) => s.session.client.port === port && s.session.captureId === captureId)!;

describe('fixtures', () => {
  it('reassembles a message split across segments with a retransmission and reordering', () => {
    const s = sessionByPort(51008, 'cap-1');
    // header (5) + count (1) + 20 entries × 8 + checksum (1)
    expect(s.streams.s2c.bytes.length).toBe(5 + 1 + 20 * 8 + 1);
    expect(s.streams.s2c.meta.segments).toHaveLength(3);
    expect(s.session.flags.retransmissions).toBe(1);
    expect(s.session.flags.outOfOrder).toBe(1);
    const messages = parseSession(SPEC_V3, sessionInput(s.session.id)!);
    expect(messages.map((m) => m.ruleId)).toEqual(['read_log_req', 'read_log_resp']);
    expect(messages[1].packetNumbers).toHaveLength(3);
    expect(messages.every((m) => m.status === 'ok')).toBe(true);
  });

  it('keeps sessions with a reused client port separate', () => {
    const reused = [...db.sessions.values()].filter((s) => s.session.client.port === 51003);
    expect(reused).toHaveLength(2);
    expect(new Set(reused.map((s) => s.session.id)).size).toBe(2);
  });

  it('marks lost bytes as unknown and keeps framing aligned after the gap', () => {
    const s = sessionByPort(53001, 'cap-3');
    expect(s.streams.s2c.meta.gaps).toEqual([{ offset: 40, length: 40, reason: 'lost' }]);
    const messages = parseSession(SPEC_V3, sessionInput(s.session.id)!).filter((m) => m.direction === 's2c');
    expect(messages.map((m) => m.status)).toEqual(['partial', 'ok']);
  });

  it('resynchronises when the beginning of a connection was not captured', () => {
    const s = sessionByPort(53004, 'cap-3');
    expect(s.session.roles).toBe('inferred');
    const { messages, unframed } = parseSessionDetailed(SPEC_V3, sessionInput(s.session.id)!);
    expect(unframed).toEqual([{ direction: 'c2s', offset: 0, length: 4 }]);
    expect(messages.filter((m) => m.direction === 'c2s').map((m) => m.ruleId)).toEqual(['get_measurement_req']);
  });
});

describe('verification runs', () => {
  it('v1 passes the main corpus but fails on the extended one', () => {
    expect(run('run-1').run.summary!.violations).toBe(0);
    expect(run('run-2').run.summary!.violations).toBe(3);
    const fields = run('run-2').findings.filter((f) => f.kind === 'violation').map((f) => f.fieldPath);
    expect(fields).toEqual(expect.arrayContaining(['value', 'temperature']));
  });

  it('v2 and v3 explain the extended corpus', () => {
    for (const id of ['run-3', 'run-5']) {
      const s = run(id).run.summary!;
      expect(s.violations).toBe(0);
      expect(s.unknownType).toBe(0);
      expect(s.unknownBytes).toBe(0);
    }
    expect(run('run-5').run.previousRunId).toBe('run-3');
  });

  it('reports capture defects instead of hiding them', () => {
    const s = run('run-4').run.summary!;
    expect(s.partial).toBeGreaterThan(0);
    expect(run('run-4').findings.some((f) => f.kind === 'capture_defect')).toBe(true);
  });
});

describe('validateSpec', () => {
  it('accepts shipped specs', () => {
    expect(validateSpec(SPEC_V2).valid).toBe(true);
  });

  it('reports structural errors with JSON paths', () => {
    const res = validateSpec({ format: 'protolab/v1', framing: { kind: 'length_prefixed', lengthField: { size: 3 } }, messages: [{ id: 'a', match: [], fields: [{ name: 'x', type: 'u7', offset: 0 }] }] });
    expect(res.valid).toBe(false);
    expect(res.issues.map((i) => i.path)).toEqual(
      expect.arrayContaining(['$.framing.lengthField.size', '$.messages[0].fields[0].type']),
    );
  });
});
