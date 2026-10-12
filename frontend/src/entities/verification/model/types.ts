import type { ISODateTime, SourceRef } from '@/shared/api';

export type RunStatus = 'queued' | 'running' | 'succeeded' | 'failed' | 'cancelled';

export type FindingKind =
  | 'violation'
  | 'partial'
  | 'ambiguous'
  | 'unknown_type'
  | 'unknown_region'
  | 'inapplicable_rule'
  | 'capture_defect';

export interface RuleStat {
  ruleId: string;
  matched: number;
  violations: number;
}

/** Covers the whole selected corpus, including everything that failed. */
export interface RunSummary {
  sessions: number;
  sessionsWithErrors: number;
  messages: number;
  ok: number;
  violations: number;
  partial: number;
  ambiguous: number;
  unknownType: number;
  /** Bytes inside framed messages not covered by any field. */
  unknownBytes: number;
  totalBytes: number;
  /** Rules that never matched any message in the corpus. */
  inapplicableRules: string[];
  byRule: RuleStat[];
}

export interface VerificationRun {
  id: string;
  projectId: string;
  interpretationId: string;
  interpretationName: string;
  versionId: string;
  version: number;
  versionHash: string;
  corpusId: string;
  corpusName: string;
  corpusHash: string;
  status: RunStatus;
  jobId?: string;
  startedAt: ISODateTime;
  finishedAt?: ISODateTime;
  durationMs?: number;
  summary?: RunSummary;
  /** Result no longer reflects the current interpretation/corpus/settings. */
  stale: boolean;
  staleReasons: string[];
  /** Previous run of the same interpretation on the same corpus — for deltas. */
  previousRunId?: string;
  settings: { overlapPolicy: string; checksumPolicy: string };
  engineVersion: string;
}

export interface Finding {
  id: string;
  runId: string;
  kind: FindingKind;
  severity: 'error' | 'warning' | 'info';
  ruleId?: string;
  fieldPath?: string;
  message: string;
  /** Total occurrences across the corpus. */
  occurrences: number;
  /** Up to N concrete examples with links to source bytes. */
  examples: SourceRef[];
}

export interface StartRunInput {
  versionId: string;
  corpusId: string;
}

export interface StartRunResult {
  run: VerificationRun;
  jobId: string;
}
