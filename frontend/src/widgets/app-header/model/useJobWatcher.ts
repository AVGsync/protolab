import { useQueryClient } from '@tanstack/react-query';
import { useEffect, useRef } from 'react';
import { isJobActive, type Job } from '@/entities/job';
import { projectKeys } from '@/entities/project';
import { toast } from '@/shared/ui';

/** Refreshes project data and notifies when a background job finishes. */
export function useJobWatcher(jobs: Job[] | undefined): void {
  const qc = useQueryClient();
  const seen = useRef(new Map<string, Job['status']>());

  useEffect(() => {
    if (!jobs) return;
    for (const job of jobs) {
      const prev = seen.current.get(job.id);
      seen.current.set(job.id, job.status);
      if (prev === undefined || !isJobActive({ status: prev }) || isJobActive(job)) continue;

      qc.invalidateQueries({ queryKey: projectKeys.root(job.projectId) });
      qc.invalidateQueries({ queryKey: projectKeys.list() });
      qc.invalidateQueries({ queryKey: ['run'] });
      if (job.status === 'succeeded') toast.success(`Готово: ${job.title}`);
      if (job.status === 'failed') toast.error(`${job.title}: ${job.error ?? 'ошибка'}`);
    }
  }, [jobs, qc]);
}
