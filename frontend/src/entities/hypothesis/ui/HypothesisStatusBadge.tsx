import { Badge } from '@/shared/ui';
import { HYPOTHESIS_STATUS } from '../model/labels';
import type { HypothesisStatus } from '../model/types';

export function HypothesisStatusBadge({ status }: { status: HypothesisStatus }) {
  const s = HYPOTHESIS_STATUS[status];
  return (
    <Badge tone={s.tone} dot title={s.description}>
      {s.label}
    </Badge>
  );
}
