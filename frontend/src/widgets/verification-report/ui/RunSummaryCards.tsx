import type { RunSummary } from '@/entities/verification';
import { formatNumber, formatPercent } from '@/shared/lib';
import { Stat } from '@/shared/ui';

function Delta({ now, before, inverse }: { now: number; before?: number; inverse?: boolean }) {
  if (before === undefined || before === now) return null;
  const diff = now - before;
  const good = inverse ? diff < 0 : diff > 0;
  return (
    <span className={good ? 'text-success' : 'text-danger'}>
      {diff > 0 ? '+' : ''}
      {formatNumber(diff)} к прошлой
    </span>
  );
}

/** The summary always covers the entire corpus: failures and unknown data are counted, not filtered out. */
export function RunSummaryCards({ summary, previous }: { summary: RunSummary; previous?: RunSummary }) {
  const coverage = summary.totalBytes ? 1 - summary.unknownBytes / summary.totalBytes : 0;
  return (
    <div className="grid grid-cols-2 gap-3 md:grid-cols-4 xl:grid-cols-8">
      <Stat label="Сессий" value={summary.sessions} hint={`с ошибками: ${summary.sessionsWithErrors}`} />
      <Stat label="Сообщений" value={formatNumber(summary.messages)} />
      <Stat label="Разобрано" value={formatNumber(summary.ok)} tone="success" hint={<Delta now={summary.ok} before={previous?.ok} />} />
      <Stat
        label="Нарушения"
        value={summary.violations}
        tone={summary.violations ? 'danger' : undefined}
        hint={<Delta now={summary.violations} before={previous?.violations} inverse />}
      />
      <Stat label="Неполные" value={summary.partial} tone={summary.partial ? 'warning' : undefined} />
      <Stat label="Неоднозначные" value={summary.ambiguous} tone={summary.ambiguous ? 'violet' : undefined} />
      <Stat
        label="Неизвестный тип"
        value={summary.unknownType}
        hint={<Delta now={summary.unknownType} before={previous?.unknownType} inverse />}
      />
      <Stat
        label="Покрытие байтов"
        value={formatPercent(coverage)}
        hint={`неописано ${formatNumber(summary.unknownBytes)} из ${formatNumber(summary.totalBytes)} Б`}
        tone={coverage === 1 ? 'success' : undefined}
      />
    </div>
  );
}
