import type { ISODateTime } from '@/shared/api';

export type ArtifactKind = 'report_html' | 'report_md' | 'interpretation' | 'results' | 'examples';

export interface ReportArtifact {
  kind: ArtifactKind;
  fileName: string;
  url: string;
  sizeBytes: number;
  sha256: string;
}

/**
 * Readable report + machine-readable interpretation + examples with expected
 * results, and the command that re-applies the interpretation to new data.
 */
export interface Report {
  id: string;
  projectId: string;
  title: string;
  createdAt: ISODateTime;
  versionId: string;
  version: number;
  interpretationName: string;
  runIds: string[];
  status: 'pending' | 'ready' | 'failed';
  artifacts: ReportArtifact[];
  reproduceCommand: string;
}

export interface CreateReportInput {
  title: string;
  versionId: string;
  runIds: string[];
}
