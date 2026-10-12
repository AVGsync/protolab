import { countLabel } from '@/shared/lib';
import type { Tone } from '@/shared/ui';
import type { RoleCertainty, SessionFlags } from './types';

export interface Anomaly {
  tone: Tone;
  /** Short phrase: "2 ретрансмиссии". */
  text: string;
  /** What it means for the analysis. */
  hint: string;
}

/** Everything unusual about a session, most severe first. A clean session has none. */
export function sessionAnomalies(flags: SessionFlags, roles?: RoleCertainty): Anomaly[] {
  const list: Anomaly[] = [];
  if (flags.gaps > 0)
    list.push({
      tone: 'danger',
      text: countLabel(flags.gaps, ['пропуск', 'пропуска', 'пропусков']),
      hint: 'В потоке нет части байтов: они показаны как неизвестные, а не как значения',
    });
  if (flags.overlapConflicts > 0)
    list.push({
      tone: 'danger',
      text: countLabel(flags.overlapConflicts, ['конфликт перекрытий', 'конфликта перекрытий', 'конфликтов перекрытий']),
      hint: 'Повтор сегмента с другими байтами: выбор по политике проекта',
    });
  if (!flags.handshake)
    list.push({ tone: 'warn', text: 'нет SYN', hint: 'Начало соединения не попало в запись' });
  if (roles === 'unknown') list.push({ tone: 'warn', text: 'роли не определены', hint: 'Клиент и сервер не различимы' });
  if (flags.retransmissions > 0)
    list.push({
      tone: 'info',
      text: countLabel(flags.retransmissions, ['ретрансмиссия', 'ретрансмиссии', 'ретрансмиссий']),
      hint: 'Повтор уже полученных байтов: в поток не добавлен',
    });
  if (flags.outOfOrder > 0)
    list.push({
      tone: 'info',
      text: countLabel(flags.outOfOrder, ['сегмент вне порядка', 'сегмента вне порядка', 'сегментов вне порядка']),
      hint: 'Сегмент пришёл раньше предшествующего; поток собран по seq',
    });
  if (roles === 'inferred')
    list.push({ tone: 'muted', text: 'роли по порту', hint: 'Роли определены по порту, а не по рукопожатию' });
  if (flags.checksumWarnings > 0)
    list.push({
      tone: 'muted',
      text: `checksum: ${flags.checksumWarnings}`,
      hint: 'Неверная контрольная сумма TCP: возможно checksum offloading на узле захвата',
    });
  return list;
}

/** One plain sentence for the context bar: "1 ретрансмиссия, 1 сегмент вне порядка". Empty when clean. */
export function describeAnomalies(anomalies: Anomaly[]): string {
  return anomalies.map((a) => a.text).join(', ');
}
