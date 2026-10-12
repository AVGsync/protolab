import type { Diagnostic } from '@/shared/api';
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

/**
 * Everything unusual about a session, most severe first. A clean session has none.
 * Diagnostics tell apart what the flags cannot: a failed reassembly, a stream cut at the size limit,
 * and damaged packets (checksum_error) among checksum offloading (checksum_offload).
 */
export function sessionAnomalies(flags: SessionFlags, roles?: RoleCertainty, diagnostics: Diagnostic[] = []): Anomaly[] {
  const diagnostic = (code: string) => diagnostics.find((d) => d.code === code);
  // The flags of a session that could not be reassembled say nothing: only this is shown.
  if (diagnostic('reassembly_failed'))
    return [{ tone: 'danger', text: 'сборка не удалась', hint: 'Внутренняя ошибка сборки соединения: пакеты сохранены, байты потоков не восстановлены' }];
  const list: Anomaly[] = [];
  if (diagnostic('stream_too_large'))
    list.push({
      tone: 'danger',
      text: 'поток обрезан пределом',
      hint: 'Поток превысил защитный предел размера: байты сверх предела не сохранены (пределы на странице «Система»)',
    });
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
  const damaged = diagnostic('checksum_error');
  const damagedCount = damaged ? (damaged.count ?? damaged.packetNumbers?.length ?? 1) : 0;
  if (damaged)
    list.push({
      tone: 'warn',
      text: countLabel(damagedCount, ['повреждённый пакет', 'повреждённых пакета', 'повреждённых пакетов']),
      hint: 'Неверная контрольная сумма у отдельных пакетов среди верных: вероятно повреждение. Их seq/ack не учитываются, байты берутся, только если нет копии с верной суммой',
    });
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
  const offloaded = flags.checksumWarnings - damagedCount;
  if (offloaded > 0)
    list.push({
      tone: 'muted',
      text: `checksum: ${offloaded}`,
      hint: 'Неверная контрольная сумма TCP: возможно checksum offloading на узле захвата, данные используются',
    });
  return list;
}

/** One plain sentence for the context bar: "1 ретрансмиссия, 1 сегмент вне порядка". Empty when clean. */
export function describeAnomalies(anomalies: Anomaly[]): string {
  return anomalies.map((a) => a.text).join(', ');
}
