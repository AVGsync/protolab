import type { ISODateTime } from '@/shared/api';

export interface CorpusFilter {
  captureIds?: string[];
  onlyTarget?: boolean;
  onlyComplete?: boolean;
  actionKinds?: string[];
}

/** A fixed set of sessions used to verify an interpretation. */
export interface Corpus {
  id: string;
  projectId: string;
  name: string;
  description: string;
  sessionIds: string[];
  /** Conditions the set was built with — shown next to every verification summary. */
  filter: CorpusFilter;
  /** Hash of the sorted session ids + capture hashes. */
  hash: string;
  createdAt: ISODateTime;
}

export interface CreateCorpusInput {
  name: string;
  description?: string;
  sessionIds: string[];
  filter: CorpusFilter;
}
