import { useQuery } from '@tanstack/react-query';
import { Activity, CheckCircle2 } from 'lucide-react';
import { useState } from 'react';
import { isJobActive, jobQueries, JobStatusBadge } from '@/entities/job';
import { CancelJobButton } from '@/features/cancel-job';
import { cn, formatDateTime, formatPercent, useClickOutside } from '@/shared/lib';
import { Progress, Spinner } from '@/shared/ui';
import { useJobWatcher } from '../model/useJobWatcher';

/** Long operations show progress, can be cancelled, and never block browsing ready data. */
export function JobsIndicator() {
  const [open, setOpen] = useState(false);
  const ref = useClickOutside<HTMLDivElement>(() => setOpen(false), open);
  const { data } = useQuery(jobQueries.list());
  useJobWatcher(data?.items);

  const jobs = data?.items ?? [];
  const active = jobs.filter(isJobActive);

  return (
    <div ref={ref} className="relative">
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className={cn(
          'inline-flex h-8 items-center gap-2 rounded-md border px-2.5 text-xs',
          active.length ? 'border-info/40 bg-info/10 text-info' : 'border-line text-muted hover:text-fg',
        )}
      >
        {active.length ? <Spinner size={13} /> : <Activity size={13} />}
        {active.length ? `Задачи: ${active.length}` : 'Задачи'}
      </button>
      {open && (
        <div className="absolute top-10 right-0 z-40 w-96 rounded-lg border border-line-strong bg-surface-2 shadow-2xl shadow-black/50">
          <div className="border-b border-line px-3 py-2 text-xs font-semibold text-muted">Фоновые задачи</div>
          {jobs.length === 0 ? (
            <p className="flex items-center gap-2 px-3 py-6 text-xs text-muted">
              <CheckCircle2 size={14} /> Задач нет
            </p>
          ) : (
            <ul className="max-h-96 divide-y divide-line overflow-y-auto">
              {jobs.map((job) => (
                <li key={job.id} className="px-3 py-2.5">
                  <div className="flex items-start justify-between gap-2">
                    <div className="min-w-0">
                      <p className="truncate text-[12.5px]">{job.title}</p>
                      <p className="text-[11px] text-subtle">
                        {isJobActive(job) ? `${job.stage ?? ''} · ${formatPercent(job.progress, 0)}` : formatDateTime(job.finishedAt ?? job.startedAt)}
                      </p>
                    </div>
                    <div className="flex items-center gap-1">
                      <CancelJobButton job={job} />
                      <JobStatusBadge status={job.status} />
                    </div>
                  </div>
                  {isJobActive(job) && <Progress value={job.progress} tone="info" className="mt-2" />}
                  {job.error && <p className="mt-1 text-[11px] text-danger">{job.error}</p>}
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </div>
  );
}
