import type { MeaningStatus } from '@/entities/message';
import type { FieldRule, InterpretationSpec } from './types';

/**
 * What backs the meaning of a field, as the engine reports it (Go spec.EffectiveMeaningStatus):
 * the explicit status, else "hypothesis" when a meaning is claimed, else "structural".
 */
export function effectiveMeaningStatus(field: Pick<FieldRule, 'status' | 'meaning'>): MeaningStatus {
  return field.status ?? (field.meaning ? 'hypothesis' : 'structural');
}

/**
 * The hypothesis a rule or field of the spec relies on: the field's own (path "a.b[].c" or "a.b[2].c"),
 * else the rule's. Used to suggest where a counterexample belongs.
 */
export function hypothesisBehind(spec: InterpretationSpec, ruleId?: string, fieldPath?: string): string | undefined {
  const rule = spec.messages.find((m) => m.id === ruleId);
  if (!rule) return undefined;
  let fields: FieldRule[] | undefined = rule.fields;
  let field: FieldRule | undefined;
  for (const name of fieldPath ? fieldPath.split('.').map((p) => p.replace(/\[\d*\]$/, '')) : []) {
    field = fields?.find((f) => f.name === name);
    fields = field?.items;
    if (!field) break;
  }
  return field?.hypothesis ?? rule.hypothesis;
}
