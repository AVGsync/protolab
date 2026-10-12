import { Clock3 } from 'lucide-react';
import { Badge, type Tone } from '@/shared/ui';
import { FINDING_KIND } from '../model/labels';
import type { FindingKind, RunStatus } from '../model/types';

const STATUS: Record<RunStatus, { tone: Tone; label: string }> = {
  queued: { tone: 'neutral', label: 'в очереди' },
  running: { tone: 'info', label: 'выполняется' },
  succeeded: { tone: 'success', label: 'завершена' },
  failed: { tone: 'danger', label: 'ошибка' },
  cancelled: { tone: 'neutral', label: 'отменена' },
};

export function RunStatusBadge({ status }: { status: RunStatus }) {
  const s = STATUS[status];
  return (
    <Badge tone={s.tone} dot>
      {s.label}
    </Badge>
  );
}

export function StaleBadge({ reasons }: { reasons: string[] }) {
  return (
    <Badge tone="warning" icon={<Clock3 size={11} />} title={reasons.join('\n')}>
      устарела
    </Badge>
  );
}

export function FindingKindBadge({ kind }: { kind: FindingKind }) {
  const k = FINDING_KIND[kind];
  return (
    <Badge tone={k.tone} title={k.description}>
      {k.label}
    </Badge>
  );
}
