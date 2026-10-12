import { Badge, type Tone } from '@/shared/ui';
import type { JobStatus } from '../model/types';

const STATUS: Record<JobStatus, { tone: Tone; label: string }> = {
  queued: { tone: 'neutral', label: 'в очереди' },
  running: { tone: 'info', label: 'выполняется' },
  succeeded: { tone: 'success', label: 'готово' },
  failed: { tone: 'danger', label: 'ошибка' },
  cancelled: { tone: 'neutral', label: 'отменено' },
};

export function JobStatusBadge({ status }: { status: JobStatus }) {
  const s = STATUS[status];
  return <Badge tone={s.tone}>{s.label}</Badge>;
}
