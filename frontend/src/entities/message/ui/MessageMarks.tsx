import { Status, type Tone } from '@/shared/ui';
import type { MeaningStatus, MessageStatus } from '../model/types';

const STATUS: Record<Exclude<MessageStatus, 'ok'>, { tone: Tone; label: string }> = {
  violation: { tone: 'danger', label: 'нарушение' },
  partial: { tone: 'warn', label: 'неполное' },
  unknown_type: { tone: 'muted', label: 'неизвестный тип' },
  ambiguous: { tone: 'warn', label: 'неоднозначно' },
};

/** A parsed message is the normal state and stays silent. */
export function MessageStatusMark({ status }: { status: MessageStatus }) {
  if (status === 'ok') return null;
  const s = STATUS[status];
  return <Status tone={s.tone}>{s.label}</Status>;
}

const MEANING: Record<MeaningStatus, { tone: Tone; label: string; title: string }> = {
  structural: { tone: 'muted', label: 'структура', title: 'Структура подтверждается правилами; смысл не утверждается' },
  hypothesis: { tone: 'warn', label: 'гипотеза', title: 'Назначение поля — непроверенное предположение' },
  confirmed: { tone: 'ok', label: 'подтверждено', title: 'Назначение проверено на корпусе и экспериментах' },
};

export function MeaningMark({ status }: { status: MeaningStatus }) {
  const m = MEANING[status];
  return (
    <Status tone={m.tone} title={m.title}>
      {m.label}
    </Status>
  );
}
