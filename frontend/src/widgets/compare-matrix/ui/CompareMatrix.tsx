import { Link } from 'react-router';
import { cn, hexByte, type ByteRange, type ColumnStat } from '@/shared/lib';

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

const KIND_CLASS = {
  constant: 'text-subtle',
  variable: 'text-glow',
  partial: 'text-violet',
};

/**
 * Messages aligned by offset. Constant columns are dimmed, variable ones are
 * highlighted — differences between actions with different parameters stand out.
 */
export function CompareMatrix({ rows, stats, highlight, paramLabel = 'параметр' }: CompareMatrixProps) {
  const inHighlight = (o: number) => highlight && o >= highlight.offset && o < highlight.offset + highlight.length;
  return (
    <div className="overflow-auto rounded-lg border border-line bg-bg">
      <table className="border-collapse font-mono text-[12px]">
        <thead className="sticky top-0 z-10 bg-surface-2">
          <tr>
            <th className="sticky left-0 z-20 min-w-64 border-r border-b border-line bg-surface-2 px-3 py-1.5 text-left font-sans text-[11px] font-semibold text-muted uppercase">
              Сообщение
            </th>
            <th className="border-r border-b border-line px-2 py-1.5 text-right font-sans text-[11px] font-semibold text-muted uppercase">
              {paramLabel}
            </th>
            {stats.map((s) => (
              <th
                key={s.offset}
                title={`смещение ${s.offset}: ${s.kind === 'constant' ? 'константа' : s.kind === 'variable' ? `${s.distinct} различных значений` : 'есть не во всех сообщениях'}`}
                className={cn(
                  'w-[26px] border-b border-line px-0 py-1.5 text-center text-[10.5px] font-normal',
                  KIND_CLASS[s.kind],
                  inHighlight(s.offset) && 'bg-accent/25 text-white',
                )}
              >
                {s.offset}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((r) => (
            <tr key={r.key} className="border-b border-line/60 hover:bg-surface/60">
              <td className="sticky left-0 z-[5] border-r border-line bg-bg px-3 py-1 font-sans">
                {r.href ? (
                  <Link to={r.href} className="text-[12.5px] hover:text-info hover:underline">
                    {r.label}
                  </Link>
                ) : (
                  <span className="text-[12.5px]">{r.label}</span>
                )}
                {r.sublabel && <div className="font-mono text-[10.5px] text-subtle">{r.sublabel}</div>}
              </td>
              <td className="border-r border-line px-2 text-right text-glow">{r.param ?? <span className="text-subtle">—</span>}</td>
              {stats.map((s) => {
                const present = s.offset < r.bytes.length;
                return (
                  <td
                    key={s.offset}
                    className={cn(
                      'px-0 text-center',
                      present ? (s.kind === 'variable' ? 'text-fg' : 'text-subtle') : 'text-line-strong',
                      inHighlight(s.offset) && 'bg-accent/25 text-white',
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
