import type { FindingKind } from './types';

export const FINDING_KIND: Record<
  FindingKind,
  { label: string; tone: 'danger' | 'warning' | 'violet' | 'neutral' | 'info'; description: string }
> = {
  violation: { label: 'нарушение', tone: 'danger', description: 'Данные противоречат правилу или условию эксперимента' },
  partial: { label: 'неполное', tone: 'warning', description: 'Сообщение затронуто пропуском или обрезано' },
  ambiguous: { label: 'неоднозначно', tone: 'violet', description: 'Подходит несколько правил' },
  unknown_type: { label: 'неизвестный тип', tone: 'neutral', description: 'Ни одно правило не подошло' },
  unknown_region: { label: 'неизвестные байты', tone: 'neutral', description: 'Байты не описаны ни одним полем' },
  inapplicable_rule: { label: 'неприменимо', tone: 'info', description: 'Правило ни разу не сработало на корпусе' },
  capture_defect: { label: 'дефект захвата', tone: 'warning', description: 'Пропуски/конфликты в исходной записи' },
};

export const FINDING_KINDS = Object.keys(FINDING_KIND) as FindingKind[];
