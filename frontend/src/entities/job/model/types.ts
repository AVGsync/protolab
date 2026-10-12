import type { ISODateTime } from '@/shared/api';

export type JobKind = 'import_capture' | 'verification' | 'report' | 'import_project' | 'reindex';
export type JobStatus = 'queued' | 'running' | 'succeeded' | 'failed' | 'cancelled';

/** Long-running operation with progress and cancellation. */
export interface Job {
  id: string;
  projectId: string;
  kind: JobKind;
  title: string;
  status: JobStatus;
  /** 0..1 */
  progress: number;
  stage?: string;
  startedAt: ISODateTime;
  finishedAt?: ISODateTime;
  error?: string;
  /** Resource produced by the job. */
  result?: { type: 'capture' | 'run' | 'report' | 'project'; id: string };
  cancellable: boolean;
}

export const isJobActive = (job: Pick<Job, 'status'>): boolean => job.status === 'queued' || job.status === 'running';
