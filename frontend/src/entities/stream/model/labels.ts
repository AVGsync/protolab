import type { GapReason } from './types';

export const GAP_REASON_LABEL: Record<GapReason, string> = {
  lost: 'потерянный сегмент',
  truncated: 'усечение при захвате',
  not_captured: 'начало не захвачено',
};
