import { Status, Tag, type Tone } from '@/shared/ui';
import { FINDING_KIND } from '../model/labels';
import type { FindingKind, RunStatus } from '../model/types';

const STATUS: Record<Exclude<RunStatus, 'succeeded'>, { tone: Tone; label: string }> = {
  queued: { tone: 'muted', label: 'в очереди' },
  running: { tone: 'info', label: 'выполняется' },
  failed: { tone: 'danger', label: 'ошибка' },
  cancelled: { tone: 'muted', label: 'отменена' },
};

/** A finished run is the normal state and stays silent. */
export function RunStatusMark({ status }: { status: RunStatus }) {
  if (status === 'succeeded') return null;
  const s = STATUS[status];
  return <Status tone={s.tone}>{s.label}</Status>;
}

export function StaleTag({ reasons }: { reasons: string[] }) {
  return <Tag title={reasons.join('\n')}>устарела</Tag>;
}

export function FindingKindMark({ kind }: { kind: FindingKind }) {
  const k = FINDING_KIND[kind];
  return (
    <Status tone={k.tone} title={k.description}>
      {k.label}
    </Status>
  );
}
