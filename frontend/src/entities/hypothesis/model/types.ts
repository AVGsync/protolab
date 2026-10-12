import type { ISODateTime, SourceRef } from '@/shared/api';

export type HypothesisStatus = 'proposed' | 'testing' | 'supported' | 'refuted';

export interface HypothesisEvent {
  at: ISODateTime;
  status: HypothesisStatus;
  note: string;
}

export interface Counterexample {
  source: SourceRef;
  note: string;
  runId?: string;
}

/** An assumption about structure or meaning, tied to observations and counterexamples. */
export interface Hypothesis {
  id: string;
  projectId: string;
  title: string;
  statement: string;
  status: HypothesisStatus;
  observationIds: string[];
  counterexamples: Counterexample[];
  /** Alternative explanations that were considered. */
  alternatives: string[];
  /** Rules/fields in interpretations that rely on this hypothesis, e.g. "set_param_req.value". */
  ruleRefs: string[];
  createdAt: ISODateTime;
  updatedAt: ISODateTime;
  history: HypothesisEvent[];
}

export interface CreateHypothesisInput {
  title: string;
  statement: string;
  observationIds?: string[];
  alternatives?: string[];
  ruleRefs?: string[];
}

export interface UpdateHypothesisInput {
  title?: string;
  statement?: string;
  status?: HypothesisStatus;
  /** Required with a status change — why the status changed. */
  note?: string;
  alternatives?: string[];
  observationIds?: string[];
  counterexamples?: Counterexample[];
}
