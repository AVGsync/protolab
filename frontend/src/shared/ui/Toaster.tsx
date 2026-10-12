import { XIcon } from '@phosphor-icons/react';
import { cn } from '../lib/cn';
import { useToastStore, type ToastTone } from './toast-store';

const SQUARE: Record<ToastTone, string> = {
  success: 'bg-ok',
  error: 'bg-danger',
  info: 'bg-info',
};

export function Toaster() {
  const items = useToastStore((s) => s.items);
  const dismiss = useToastStore((s) => s.dismiss);

  return (
    <div className="pointer-events-none fixed right-4 bottom-10 z-[60] flex w-80 flex-col gap-2">
      {items.map((t) => (
        <div
          key={t.id}
          role="status"
          className="pointer-events-auto flex items-start gap-2.5 rounded-ctl border border-line-strong bg-surface px-3 py-2 shadow-pop"
        >
          <i aria-hidden className={cn('mt-[7px] size-[7px] shrink-0', SQUARE[t.tone])} />
          <p className="flex-1 text-sm">{t.message}</p>
          <button type="button" onClick={() => dismiss(t.id)} className="mt-0.5 text-muted hover:text-fg" aria-label="Закрыть">
            <XIcon size={12} />
          </button>
        </div>
      ))}
    </div>
  );
}
