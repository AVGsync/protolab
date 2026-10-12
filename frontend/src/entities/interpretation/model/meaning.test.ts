import { describe, expect, it } from 'vitest';
import type { InterpretationSpec } from './types';
import { effectiveMeaningStatus, hypothesisBehind } from './meaning';

describe('effectiveMeaningStatus', () => {
  it('treats a claimed meaning without a status as a hypothesis, as the engine does', () => {
    expect(effectiveMeaningStatus({ meaning: 'температура' })).toBe('hypothesis');
    expect(effectiveMeaningStatus({})).toBe('structural');
    expect(effectiveMeaningStatus({ meaning: 'температура', status: 'confirmed' })).toBe('confirmed');
    expect(effectiveMeaningStatus({ meaning: 'x', status: 'structural' })).toBe('structural');
  });
});

describe('hypothesisBehind', () => {
  const spec: InterpretationSpec = {
    format: 'protolab/v1',
    protocol: 'P',
    framing: { kind: 'fixed', size: 4 },
    messages: [
      {
        id: 'log',
        hypothesis: 'h-rule',
        match: [],
        fields: [
          { name: 'count', offset: 0, type: 'u8' },
          { name: 'entries', offset: 1, type: 'array', count: 'count', itemSize: 2, items: [{ name: 'value', offset: 0, type: 'i16', hypothesis: 'h-value' }] },
        ],
      },
    ],
  };
  it('prefers the field hypothesis, also inside arrays', () => {
    expect(hypothesisBehind(spec, 'log', 'entries[].value')).toBe('h-value');
    expect(hypothesisBehind(spec, 'log', 'entries[3].value')).toBe('h-value');
  });
  it('falls back to the rule and knows nothing of other rules', () => {
    expect(hypothesisBehind(spec, 'log', 'count')).toBe('h-rule');
    expect(hypothesisBehind(spec, 'log')).toBe('h-rule');
    expect(hypothesisBehind(spec, 'other', 'count')).toBeUndefined();
  });
});
