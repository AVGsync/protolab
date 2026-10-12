import { AlertOctagon, AlertTriangle, Info } from 'lucide-react';
import type { Diagnostic } from '@/shared/api';
import { cn } from '@/shared/lib';

const ICON = {
  info: <Info size={14} className="text-info" />,
  warning: <AlertTriangle size={14} className="text-warning" />,
  error: <AlertOctagon size={14} className="text-danger" />,
};

export function DiagnosticList({ items, className }: { items: Diagnostic[]; className?: string }) {
  if (items.length === 0) return <p className={cn('text-xs text-subtle', className)}>Диагностик нет.</p>;
  return (
    <ul className={cn('flex flex-col gap-1.5', className)}>
      {items.map((d, i) => (
        <li key={`${d.code}-${i}`} className="flex items-start gap-2 text-[12.5px]">
          <span className="mt-0.5">{ICON[d.level]}</span>
          <span className="flex-1">
            {d.message}
            {d.count !== undefined && <span className="ml-1 text-muted">×{d.count}</span>}
            {d.packetNumbers && d.packetNumbers.length > 0 && (
              <span className="ml-1.5 font-mono text-[11px] text-subtle">
                пакеты #{d.packetNumbers.slice(0, 6).join(', #')}
                {d.packetNumbers.length > 6 && '…'}
              </span>
            )}
          </span>
          <code className="font-mono text-[10.5px] text-subtle">{d.code}</code>
        </li>
      ))}
    </ul>
  );
}
