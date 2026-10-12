import { cn, formatNumber } from '@/shared/lib';
import { StackBar } from '@/shared/ui';
import type { RunSummary } from '../model/types';

/** Parts of the corpus outcome, in reading order: fine first, then what needs a look. */
function outcomeParts(s: RunSummary) {
  return [
    { value: s.ok, className: 'bg-ok', text: 'text-ok', label: 'разобрано' },
    { value: s.violations, className: 'bg-danger', text: 'text-danger', label: 'нарушений' },
    { value: s.partial + s.ambiguous, className: 'bg-warn', text: 'text-warn', label: 'неполных или неоднозначных' },
    { value: s.unknownType, className: 'bg-unknown', text: 'text-muted', label: 'неизвестного типа' },
  ];
}

export interface RunOutcomeProps {
  summary: RunSummary;
  className?: string;
}

/**
 * One strip per run: the share of messages that parse, violate a rule, are partial or have an unknown type.
 * Counts of the quiet part (parsed) stay in the tooltip; only the exceptions are written next to the strip.
 */
export function RunOutcome({ summary, className }: RunOutcomeProps) {
  const parts = outcomeParts(summary);
  const exceptions = parts.slice(1).filter((p) => p.value > 0);
  const title = parts.map((p) => `${p.label}: ${formatNumber(p.value)}`).join('\n');
  return (
    <div className={cn('flex items-center gap-3', className)} title={title}>
      <span className="w-6 shrink-0 text-right font-mono text-xs">{formatNumber(summary.messages)}</span>
      <StackBar
        className="w-28"
        total={Math.max(summary.messages, parts.reduce((a, p) => a + p.value, 0))}
        parts={parts.map((p) => ({ value: p.value, className: p.className }))}
        aria-label={title}
      />
      {exceptions.length > 0 && (
        <span className="flex items-center gap-2 font-mono text-xs">
          {exceptions.map((p) => (
            <span key={p.label} className={p.text}>
              {p.value}
            </span>
          ))}
        </span>
      )}
    </div>
  );
}
