import { Status, type Tone } from '@/shared/ui';
import type { JobStatus } from '../model/types';

const STATUS: Record<Exclude<JobStatus, 'succeeded'>, { tone: Tone; label: string }> = {
  queued: { tone: 'muted', label: 'в очереди' },
  running: { tone: 'info', label: 'выполняется' },
  failed: { tone: 'danger', label: 'ошибка' },
  cancelled: { tone: 'muted', label: 'отменено' },
};

export function JobStatusMark({ status }: { status: JobStatus }) {
  if (status === 'succeeded') return <span className="text-muted">готово</span>;
  const s = STATUS[status];
  return <Status tone={s.tone}>{s.label}</Status>;
}
