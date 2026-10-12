import type { Job, JobKind } from '@/entities/job';
import { nextId } from './db';

interface MockJob {
  job: Job;
  startedAtMs: number;
  durationMs: number;
  stages: string[];
  onDone?: () => Job['result'] | void;
  onCancel?: () => void;
}

const jobs: MockJob[] = [];

export interface StartJobOptions {
  projectId: string;
  kind: JobKind;
  title: string;
  durationMs: number;
  stages: string[];
  onDone?: () => Job['result'] | void;
  onCancel?: () => void;
}

export function startJob(opts: StartJobOptions): Job {
  const job: Job = {
    id: nextId('job'),
    projectId: opts.projectId,
    kind: opts.kind,
    title: opts.title,
    status: 'running',
    progress: 0,
    stage: opts.stages[0],
    startedAt: new Date().toISOString(),
    cancellable: true,
  };
  jobs.unshift({ job, startedAtMs: Date.now(), durationMs: opts.durationMs, stages: opts.stages, onDone: opts.onDone, onCancel: opts.onCancel });
  return job;
}

/** Advances simulated jobs; called at the beginning of every mock request. */
export function tickJobs(): void {
  const now = Date.now();
  for (const j of jobs) {
    if (j.job.status !== 'running') continue;
    const progress = Math.min(1, (now - j.startedAtMs) / j.durationMs);
    j.job.progress = progress;
    j.job.stage = j.stages[Math.min(j.stages.length - 1, Math.floor(progress * j.stages.length))];
    if (progress >= 1) {
      try {
        const result = j.onDone?.();
        if (result) j.job.result = result;
        j.job.status = 'succeeded';
      } catch (err) {
        j.job.status = 'failed';
        j.job.error = err instanceof Error ? err.message : String(err);
      }
      j.job.finishedAt = new Date().toISOString();
      j.job.cancellable = false;
    }
  }
}

export function cancelJob(id: string): Job | undefined {
  const j = jobs.find((x) => x.job.id === id);
  if (!j || j.job.status !== 'running') return j?.job;
  j.job.status = 'cancelled';
  j.job.finishedAt = new Date().toISOString();
  j.job.cancellable = false;
  j.onCancel?.();
  return j.job;
}

export function listJobs(limit = 20): Job[] {
  return jobs.slice(0, limit).map((j) => ({ ...j.job }));
}

export function getJob(id: string): Job | undefined {
  const j = jobs.find((x) => x.job.id === id);
  return j && { ...j.job };
}
