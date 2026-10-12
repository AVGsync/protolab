import type { MeaningStatus } from '@/entities/message';
import type { Direction, ISODateTime } from '@/shared/api';

export type Endian = 'le' | 'be';
export type FieldType = 'u8' | 'i8' | 'u16' | 'i16' | 'u32' | 'i32' | 'bytes' | 'ascii' | 'array';

export interface FieldExpect {
  equals?: number;
  min?: number;
  max?: number;
  oneOf?: number[];
}

export interface FieldRule {
  name: string;
  /** Offset within the message (or array item). Negative values count from the message end. */
  offset: number;
  type: FieldType;
  endian?: Endian;
  /** bytes/ascii: fixed length or the name of a previous field holding the length. */
  length?: number | string;
  /** array: name of the field holding the item count. */
  count?: string;
  itemSize?: number;
  items?: FieldRule[];
  /** Display value = raw × scale. */
  scale?: number;
  unit?: string;
  enum?: Record<string, string>;
  expect?: FieldExpect;
  /** Treat the field as a checksum over message bytes [0, field offset). */
  checksum?: 'sum8' | 'xor8';
  meaning?: string;
  status?: MeaningStatus;
  /** Hypothesis id backing the meaning. */
  hypothesis?: string;
}

export interface MatchCondition {
  offset: number;
  /** Expected bytes, hex. */
  hex: string;
}

export interface MessageRule {
  id: string;
  title?: string;
  direction?: Direction | 'any';
  /** All conditions must hold. */
  match: MatchCondition[];
  fields: FieldRule[];
  hypothesis?: string;
}

export type FramingRule =
  | {
      kind: 'length_prefixed';
      lengthField: { offset: number; size: 1 | 2 | 4; endian: Endian };
      /** total message length = length field value + lengthAdjust */
      lengthAdjust: number;
      minLength?: number;
      maxLength?: number;
      /** Hex pattern to resynchronise on after a gap or framing error. */
      sync?: string;
    }
  | { kind: 'delimiter'; delimiter: string; sync?: string }
  | { kind: 'fixed'; size: number };

/** Check against experiment conditions recorded in the action log. */
export interface ActionCheck {
  field: string;
  /** Path inside the linked action: "params.<name>" or "observed.<name>". */
  equals: string;
}

export interface ActionAssertion {
  id: string;
  rule: string;
  actionKind: string;
  checks: ActionCheck[];
  hypothesis?: string;
}

/** Declarative, machine-readable protocol description (format "protolab/v1"). */
export interface InterpretationSpec {
  format: 'protolab/v1';
  protocol: string;
  transport?: { port?: number };
  framing: FramingRule;
  messages: MessageRule[];
  assertions?: ActionAssertion[];
}

export interface Interpretation {
  id: string;
  projectId: string;
  name: string;
  description: string;
  latestVersion: number;
  latestVersionId: string;
  createdAt: ISODateTime;
  updatedAt: ISODateTime;
}

/** Versions are immutable: every change produces a new version with its own hash. */
export interface InterpretationVersion {
  id: string;
  interpretationId: string;
  version: number;
  /** SHA-256 of the canonical JSON spec. */
  hash: string;
  comment: string;
  createdAt: ISODateTime;
  parentVersionId?: string;
  spec: InterpretationSpec;
}

export interface SpecIssue {
  path: string;
  message: string;
  severity: 'error' | 'warning';
}

export interface ValidationResult {
  valid: boolean;
  issues: SpecIssue[];
}

export interface CreateInterpretationInput {
  name: string;
  description?: string;
  spec: InterpretationSpec;
}
