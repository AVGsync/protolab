import { Link } from 'react-router';
import { cn, columnHeat, hexByte, type ByteRange, type ColumnStat } from '@/shared/lib';

export interface CompareRowData {
  key: string;
  label: string;
  sublabel?: string;
  bytes: Uint8Array;
  param?: number | null;
  href?: string;
}

export interface CompareMatrixProps {
  rows: CompareRowData[];
  stats: ColumnStat[];
  highlight?: ByteRange | null;
  paramLabel?: string;
}

const HEAT = ['bg-heat-0', 'bg-heat-1', 'bg-heat-2', 'bg-heat-3', 'bg-heat-4', 'bg-heat-5'] as const;

/**
 * Messages aligned by offset. Every column carries a small variability bar (the heat scale): constant bytes stay
 * quiet, bytes that follow the experiment's parameter light up, so the eye finds candidate fields without reading
 * the hex. Columns that are missing in some messages are hatched.
 */
export function CompareMatrix({ rows, stats, highlight, paramLabel = 'параметр' }: CompareMatrixProps) {
  const inHighlight = (o: number) => highlight && o >= highlight.offset && o < highlight.offset + highlight.length;
  return (
    <div className="overflow-auto border border-line bg-bg">
      <table className="border-collapse font-mono text-xs">
        <thead className="sticky top-0 z-10 bg-surface">
          <tr>
            <th className="legend sticky left-0 z-20 min-w-64 border-r border-b border-line bg-surface px-3 py-1.5 text-left font-sans font-normal text-muted">
              Сообщение
            </th>
            <th className="legend border-r border-b border-line px-3 py-1.5 text-right font-sans font-normal text-muted">{paramLabel}</th>
            {stats.map((s) => (
              <th
                key={s.offset}
                title={`смещение ${s.offset}: ${s.kind === 'constant' ? 'константа' : s.kind === 'variable' ? `${s.distinct} различных значений` : 'есть не во всех сообщениях'}`}
                className={cn(
                  'w-[28px] border-b border-line px-0 pt-1.5 pb-0 text-center text-2xs font-normal',
                  s.kind === 'constant' ? 'text-muted' : 'text-fg',
                  inHighlight(s.offset) && 'bg-selection',
                )}
              >
                {s.offset}
                <i aria-hidden className={cn('mt-1 block h-1', s.kind === 'partial' ? 'bg-hatch bg-line' : HEAT[columnHeat(s)])} />
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((r) => (
            <tr key={r.key} className="border-b border-line/60 hover:bg-surface">
              <td className="sticky left-0 z-[5] border-r border-line bg-bg px-3 py-1 font-sans">
                {r.href ? (
                  <Link to={r.href} className="text-xs hover:link">
                    {r.label}
                  </Link>
                ) : (
                  <span className="text-xs">{r.label}</span>
                )}
                {r.sublabel && <div className="font-mono text-2xs text-muted">{r.sublabel}</div>}
              </td>
              <td className="border-r border-line px-3 text-right text-fg">{r.param ?? <span className="text-muted">—</span>}</td>
              {stats.map((s) => {
                const present = s.offset < r.bytes.length;
                return (
                  <td
                    key={s.offset}
                    className={cn(
                      'px-0 text-center',
                      present ? (s.kind === 'constant' ? 'text-muted' : 'text-fg') : 'text-line-strong',
                      inHighlight(s.offset) && 'bg-selection text-fg',
                    )}
                  >
                    {present ? hexByte(r.bytes[s.offset]) : '··'}
                  </td>
                );
              })}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

/** Key to the bar under the column numbers. */
export function HeatLegend({ className }: { className?: string }) {
  return (
    <p className={cn('flex flex-wrap items-center gap-x-3 gap-y-1 text-2xs text-muted', className)}>
      <span>Полоска под номером столбца: изменчивость</span>
      <span className="flex items-center gap-1.5">
        константа
        <span aria-hidden className="flex gap-px">
          {HEAT.map((h) => (
            <i key={h} className={cn('h-1 w-4', h)} />
          ))}
        </span>
        всегда разные
      </span>
      <span className="flex items-center gap-1.5">
        <i aria-hidden className="bg-hatch h-1 w-4 bg-line" />
        есть не во всех сообщениях
      </span>
    </p>
  );
}
