import { useQuery } from '@tanstack/react-query';
import { useState } from 'react';
import { isJobActive, jobQueries, JobStatusMark } from '@/entities/job';
import { CancelJobButton } from '@/features/cancel-job';
import { cn, formatDateTime, formatPercent, useClickOutside } from '@/shared/lib';
import { Progress } from '@/shared/ui';
import { useJobWatcher } from '../model/useJobWatcher';

/** Long operations show progress, can be cancelled, and never block browsing ready data. */
export function JobsIndicator() {
  const [open, setOpen] = useState(false);
  const ref = useClickOutside<HTMLDivElement>(() => setOpen(false), open);
  const { data } = useQuery(jobQueries.list());
  useJobWatcher(data?.items);

  const jobs = data?.items ?? [];
  const active = jobs.filter(isJobActive);
  const first = active[0];

  // An empty "Задач нет" is permanent noise: the indicator exists only while there is something to show.
  if (jobs.length === 0) return null;

  return (
    <div ref={ref} className="relative">
      <button
        type="button"
        onClick={() => setOpen(!open)}
        aria-expanded={open}
        className={cn('flex h-6 items-center gap-2 rounded-ctl px-1.5 hover:bg-surface-2', first ? 'text-fg' : 'text-muted hover:text-fg')}
      >
        {first ? (
          <>
            <i aria-hidden className="size-[7px] animate-pulse bg-accent" />
            <span className="max-w-56 truncate">{first.title}</span>
            <span className="font-mono">{formatPercent(first.progress, 0)}</span>
            {active.length > 1 && <span className="font-mono text-muted">+{active.length - 1}</span>}
          </>
        ) : (
          'Задачи'
        )}
      </button>
      {open && (
        <div className="absolute top-8 right-0 z-40 w-[28rem] rounded-ctl border border-line-strong bg-surface text-fg shadow-pop">
          <div className="border-b border-line px-3 py-1.5 text-xs text-muted">Фоновые задачи</div>
          {jobs.length === 0 ? (
            <p className="px-3 py-6 text-center text-xs text-muted">Задач нет</p>
          ) : (
            <ul className="max-h-96 divide-y divide-line overflow-y-auto">
              {jobs.map((job) => (
                <li key={job.id} className="px-3 py-2">
                  <div className="flex items-start justify-between gap-2">
                    <div className="min-w-0">
                      <p className="truncate text-sm">{job.title}</p>
                      <p className="font-mono text-2xs text-muted">
                        {isJobActive(job) ? `${job.stage ?? ''} ${formatPercent(job.progress, 0)}` : formatDateTime(job.finishedAt ?? job.startedAt)}
                      </p>
                    </div>
                    <div className="flex items-center gap-2 text-xs">
                      <CancelJobButton job={job} />
                      <JobStatusMark status={job.status} />
                    </div>
                  </div>
                  {isJobActive(job) && <Progress value={job.progress} className="mt-2" />}
                  {job.error && <p className="mt-1 text-xs text-danger">{job.error}</p>}
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </div>
  );
}
