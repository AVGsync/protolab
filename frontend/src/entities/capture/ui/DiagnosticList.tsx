import type { Diagnostic, DiagnosticLevel } from '@/shared/api';
import { cn } from '@/shared/lib';
import { Status, type Tone } from '@/shared/ui';

const TONE: Record<DiagnosticLevel, Tone> = { info: 'info', warning: 'warn', error: 'danger' };

export function DiagnosticList({ items, className }: { items: Diagnostic[]; className?: string }) {
  if (items.length === 0) return <p className={cn('text-xs text-muted', className)}>Диагностик нет.</p>;
  return (
    <ul className={cn('flex flex-col gap-1', className)}>
      {items.map((d, i) => (
        <li key={`${d.code}-${i}`} className="flex items-start gap-3 text-sm">
          <Status tone={TONE[d.level]} className="items-start [&>i]:mt-[7px]">
            <span className="whitespace-normal">
              {d.message}
              {d.count !== undefined && <span className="ml-1 text-muted">x{d.count}</span>}
              {d.packetNumbers && d.packetNumbers.length > 0 && (
                <span className="ml-1.5 font-mono text-2xs text-muted">
                  пакеты #{d.packetNumbers.slice(0, 6).join(', #')}
                  {d.packetNumbers.length > 6 && '…'}
                </span>
              )}
            </span>
          </Status>
          <code className="ml-auto font-mono text-2xs text-muted">{d.code}</code>
        </li>
      ))}
    </ul>
  );
}
