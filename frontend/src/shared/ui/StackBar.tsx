import { cn } from '../lib/cn';

export interface StackBarPart {
  value: number;
  /** Fill colour class, e.g. `bg-ok`. */
  className: string;
  title?: string;
}

export interface StackBarProps {
  parts: readonly StackBarPart[];
  /** Whole of the bar; defaults to the sum of the parts. */
  total?: number;
  className?: string;
  'aria-label'?: string;
}

/**
 * Proportions of one whole as a flat strip (a sparkline-sized bar chart): the outcome of a run at a glance,
 * where a row of five numbers would be read one by one. Empty parts take no room.
 */
export function StackBar({ parts, total, className, 'aria-label': ariaLabel }: StackBarProps) {
  const sum = total ?? parts.reduce((acc, p) => acc + p.value, 0);
  return (
    <div role="img" aria-label={ariaLabel} className={cn('flex h-1.5 w-full gap-px overflow-hidden bg-line', className)}>
      {sum > 0 &&
        parts
          .filter((p) => p.value > 0)
          .map((p, i) => (
            <div key={i} title={p.title} className={p.className} style={{ width: `${(p.value / sum) * 100}%`, minWidth: 2 }} />
          ))}
    </div>
  );
}
