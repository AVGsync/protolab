import { Badge, type Tone } from '@/shared/ui';
import type { MeaningStatus, MessageStatus } from '../model/types';

const STATUS: Record<MessageStatus, { tone: Tone; label: string }> = {
  ok: { tone: 'success', label: 'разобрано' },
  violation: { tone: 'danger', label: 'нарушение' },
  partial: { tone: 'warning', label: 'неполное' },
  unknown_type: { tone: 'neutral', label: 'неизвестный тип' },
  ambiguous: { tone: 'violet', label: 'неоднозначно' },
};

export function MessageStatusBadge({ status }: { status: MessageStatus }) {
  const s = STATUS[status];
  return <Badge tone={s.tone}>{s.label}</Badge>;
}

const MEANING: Record<MeaningStatus, { tone: Tone; label: string; title: string }> = {
  structural: { tone: 'info', label: 'структура', title: 'Структура подтверждается правилами; смысл не утверждается' },
  hypothesis: { tone: 'warning', label: 'гипотеза', title: 'Назначение поля — непроверенное предположение' },
  confirmed: { tone: 'success', label: 'подтверждено', title: 'Назначение проверено на корпусе и экспериментах' },
};

export function MeaningBadge({ status }: { status: MeaningStatus }) {
  const m = MEANING[status];
  return (
    <Badge tone={m.tone} title={m.title}>
      {m.label}
    </Badge>
  );
}
