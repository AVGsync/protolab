import { Status } from '@/shared/ui';
import { HYPOTHESIS_STATUS } from '../model/labels';
import type { HypothesisStatus } from '../model/types';

export function HypothesisStatusMark({ status }: { status: HypothesisStatus }) {
  const s = HYPOTHESIS_STATUS[status];
  return (
    <Status tone={s.tone} title={s.description}>
      {s.label}
    </Status>
  );
}
