import type { RunSummary } from '@/entities/verification';
import { cn, formatNumber, formatPercent } from '@/shared/lib';
import { Readout, StackBar } from '@/shared/ui';

function Delta({ now, before, inverse }: { now: number; before?: number; inverse?: boolean }) {
  if (before === undefined || before === now) return null;
  const diff = now - before;
  const good = inverse ? diff < 0 : diff > 0;
  return (
    <span className={cn('ml-1', good ? 'text-ok' : 'text-danger')} title="Изменение относительно предыдущей проверки">
      {diff > 0 ? '+' : ''}
      {formatNumber(diff)}
    </span>
  );
}

/**
 * The summary always covers the entire corpus: failures and unknown data are counted, not filtered out.
 * One strip and one headline number; every category is listed, zeros muted, so "no violations" is a visible result.
 */
export function RunReadout({ summary, previous }: { summary: RunSummary; previous?: RunSummary }) {
  const coverage = summary.totalBytes ? 1 - summary.unknownBytes / summary.totalBytes : 0;
  const categories = [
    { label: 'разобрано', value: summary.ok, before: previous?.ok, bar: 'bg-ok', inverse: false, tone: 'text-fg' },
    { label: 'нарушений', value: summary.violations, before: previous?.violations, bar: 'bg-danger', inverse: true, tone: 'text-danger' },
    { label: 'неполных', value: summary.partial, before: previous?.partial, bar: 'bg-warn', inverse: true, tone: 'text-warn' },
    { label: 'неоднозначных', value: summary.ambiguous, before: previous?.ambiguous, bar: 'bg-warn', inverse: true, tone: 'text-warn' },
    { label: 'неизвестный тип', value: summary.unknownType, before: previous?.unknownType, bar: 'bg-unknown', inverse: true, tone: 'text-fg' },
  ];
  const total = Math.max(summary.messages, categories.reduce((acc, c) => acc + c.value, 0));

  return (
    <section className="border-y border-line py-3">
      <div className="flex flex-wrap items-baseline justify-between gap-x-6 gap-y-1">
        <p>
          <span className="font-mono text-xl font-semibold">{formatNumber(summary.ok)}</span>
          <span className="text-muted"> из {formatNumber(summary.messages)} сообщений разобрано</span>
          <Delta now={summary.ok} before={previous?.ok} />
        </p>
        <Readout
          items={[
            { label: 'сессий', value: summary.sessionsWithErrors ? `${summary.sessions}, с ошибками ${summary.sessionsWithErrors}` : summary.sessions },
            {
              label: 'покрытие полями',
              value: (
                <span title={`не описано ${formatNumber(summary.unknownBytes)} из ${formatNumber(summary.totalBytes)} Б`}>{formatPercent(coverage)}</span>
              ),
            },
          ]}
        />
      </div>
      <StackBar className="mt-3 h-2" total={total} parts={categories.map((c) => ({ value: c.value, className: c.bar, title: `${c.label}: ${c.value}` }))} />
      <ul className="mt-2.5 flex flex-wrap gap-x-6 gap-y-1 text-xs">
        {categories.map((c) => (
          <li key={c.label} className={cn('flex items-center gap-1.5', c.value > 0 ? 'text-fg' : 'text-muted')}>
            <i aria-hidden className={cn('size-[7px]', c.value > 0 ? c.bar : 'bg-line-strong')} />
            {c.label}
            <span className={cn('font-mono', c.value > 0 && c.tone)}>
              {c.value}
              <Delta now={c.value} before={c.before} inverse={c.inverse} />
            </span>
          </li>
        ))}
      </ul>
    </section>
  );
}
