import type { ISODateTime, SourceRef } from '@/shared/api';

/**
 * A researcher's note bound to exact source bytes. `snapshotHex` keeps the
 * bytes seen at creation so re-binding to other bytes is detected (integrity).
 */
export interface Observation {
  id: string;
  projectId: string;
  note: string;
  source: SourceRef;
  snapshotHex: string;
  integrity: 'ok' | 'mismatch';
  hypothesisIds: string[];
  tags: string[];
  createdAt: ISODateTime;
}

export interface CreateObservationInput {
  note: string;
  source: SourceRef;
  hypothesisIds?: string[];
  tags?: string[];
}

export interface ObservationFilters {
  hypothesisId?: string;
  sessionId?: string;
}
