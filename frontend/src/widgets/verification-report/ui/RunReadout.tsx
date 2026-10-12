import type { RunSummary } from '@/entities/verification';
import { formatNumber, formatPercent } from '@/shared/lib';
import { Readout } from '@/shared/ui';

function Delta({ now, before, inverse }: { now: number; before?: number; inverse?: boolean }) {
  if (before === undefined || before === now) return null;
  const diff = now - before;
  const good = inverse ? diff < 0 : diff > 0;
  return (
    <span className={good ? 'text-ok' : 'text-danger'} title="Изменение относительно предыдущей проверки">
      {' '}
      {diff > 0 ? '+' : ''}
      {formatNumber(diff)}
    </span>
  );
}

/** The summary always covers the entire corpus: failures and unknown data are counted, not filtered out. */
export function RunReadout({ summary, previous }: { summary: RunSummary; previous?: RunSummary }) {
  const coverage = summary.totalBytes ? 1 - summary.unknownBytes / summary.totalBytes : 0;
  return (
    <Readout
      className="border-y border-line py-2"
      items={[
        { label: 'сессий', value: summary.sessionsWithErrors ? `${summary.sessions} (с ошибками ${summary.sessionsWithErrors})` : summary.sessions },
        { label: 'сообщений', value: formatNumber(summary.messages) },
        {
          label: 'разобрано',
          value: (
            <>
              {formatNumber(summary.ok)}
              <Delta now={summary.ok} before={previous?.ok} />
            </>
          ),
        },
        {
          label: 'нарушений',
          value: (
            <span className={summary.violations ? 'text-danger' : undefined}>
              {summary.violations}
              <Delta now={summary.violations} before={previous?.violations} inverse />
            </span>
          ),
        },
        { label: 'неполных', value: <span className={summary.partial ? 'text-warn' : undefined}>{summary.partial}</span> },
        { label: 'неоднозначных', value: <span className={summary.ambiguous ? 'text-warn' : undefined}>{summary.ambiguous}</span> },
        {
          label: 'неизвестный тип',
          value: (
            <>
              {summary.unknownType}
              <Delta now={summary.unknownType} before={previous?.unknownType} inverse />
            </>
          ),
        },
        {
          label: 'покрытие',
          value: (
            <span title={`не описано ${formatNumber(summary.unknownBytes)} из ${formatNumber(summary.totalBytes)} Б`}>
              {formatPercent(coverage)}
            </span>
          ),
        },
      ]}
    />
  );
}
