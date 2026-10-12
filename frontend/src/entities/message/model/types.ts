import type { Direction, ISODateTime } from '@/shared/api';

export type MessageStatus = 'ok' | 'violation' | 'partial' | 'unknown_type' | 'ambiguous';
export type FieldStatus = 'ok' | 'violation' | 'unknown';

/**
 * What backs a field's meaning: structure confirmed by rules, an open
 * hypothesis, or a confirmed interpretation. A successful parse alone does not
 * confirm the meaning.
 */
export type MeaningStatus = 'structural' | 'hypothesis' | 'confirmed';

export interface ParsedField {
  /** Dotted path, e.g. "entries[2].value". */
  path: string;
  name: string;
  /** Offset within the message. */
  offset: number;
  length: number;
  type: string;
  /** Raw decoded value. */
  value: number | string | null;
  /** Human-readable value with scale/unit/enum applied. */
  display: string;
  status: FieldStatus;
  issue?: string;
  meaning?: string;
  meaningStatus?: MeaningStatus;
  hypothesisId?: string;
  children?: ParsedField[];
}

/** An application message produced by the framing rule of an interpretation. */
export interface Message {
  sessionId: string;
  direction: Direction;
  /** Ordinal of the message in the session (both directions, by time). */
  index: number;
  /** Offset in the directed stream. */
  offset: number;
  length: number;
  timestamp: ISODateTime;
  packetNumbers: number[];
  ruleId: string | null;
  ruleTitle?: string;
  /** Set when several rules match (ambiguous). */
  candidateRuleIds?: string[];
  status: MessageStatus;
  issues: string[];
  fields: ParsedField[];
  /** Message-relative byte ranges not covered by any field. */
  unknown: { offset: number; length: number }[];
  /** Message bytes, base64. Unknown (gap) bytes are listed in `unknown` and `issues`. */
  data: string;
}

export interface ProjectMessagesQuery {
  versionId: string;
  ruleId?: string;
  direction?: Direction;
  sessionIds?: string[];
}
