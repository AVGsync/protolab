import { CheckCircle2, Info, X, XCircle } from 'lucide-react';
import { cn } from '../lib/cn';
import { useToastStore } from './toast-store';

const ICONS = {
  success: <CheckCircle2 size={16} className="text-success" />,
  error: <XCircle size={16} className="text-danger" />,
  info: <Info size={16} className="text-info" />,
};

export function Toaster() {
  const items = useToastStore((s) => s.items);
  const dismiss = useToastStore((s) => s.dismiss);

  return (
    <div className="pointer-events-none fixed right-4 bottom-4 z-[60] flex w-80 flex-col gap-2">
      {items.map((t) => (
        <div
          key={t.id}
          role="status"
          className={cn(
            'pointer-events-auto flex items-start gap-2.5 rounded-lg border bg-surface-2 px-3 py-2.5 shadow-xl shadow-black/40',
            t.tone === 'error' ? 'border-danger/40' : 'border-line-strong',
          )}
        >
          <div className="mt-0.5">{ICONS[t.tone]}</div>
          <p className="flex-1 text-[12.5px]">{t.message}</p>
          <button type="button" onClick={() => dismiss(t.id)} className="text-subtle hover:text-fg" aria-label="Закрыть">
            <X size={14} />
          </button>
        </div>
      ))}
    </div>
  );
}
