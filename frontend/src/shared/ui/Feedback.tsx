import { CircleNotchIcon } from '@phosphor-icons/react';
import { useEffect, useState, type ReactNode } from 'react';
import { ApiError } from '../api/http';
import { cn } from '../lib/cn';
import { Button } from './Button';
import type { Tone } from './Status';

export function Spinner({ size = 16, className }: { size?: number; className?: string }) {
  return <CircleNotchIcon size={size} className={cn('animate-spin', className)} aria-hidden />;
}

/**
 * Waits `delay` ms before it shows anything: answers that arrive within a blink never flash a spinner.
 * The space is reserved meanwhile, so the page does not jump.
 */
export function Loader({ label = 'Загрузка…', className, delay = 200 }: { label?: string; className?: string; delay?: number }) {
  const [visible, setVisible] = useState(delay <= 0);
  useEffect(() => {
    if (delay <= 0) return;
    const timer = setTimeout(() => setVisible(true), delay);
    return () => clearTimeout(timer);
  }, [delay]);
  return (
    <div role="status" aria-busy className={cn('flex items-center justify-center gap-2 py-10 text-muted', className)}>
      {visible && (
        <>
          <Spinner size={14} />
          <span>{label}</span>
        </>
      )}
    </div>
  );
}

export interface EmptyStateProps {
  title: ReactNode;
  description?: ReactNode;
  action?: ReactNode;
  className?: string;
}

/** An empty state is a plain statement and, when there is one, the next action: no illustration. */
export function EmptyState({ title, description, action, className }: EmptyStateProps) {
  return (
    <div className={cn('flex flex-col items-center justify-center px-6 py-12 text-center', className)}>
      <p className="font-medium">{title}</p>
      {description && <p className="mt-1 max-w-md text-xs text-muted">{description}</p>}
      {action && <div className="mt-4">{action}</div>}
    </div>
  );
}

export function ErrorState({ error, onRetry, className }: { error: unknown; onRetry?: () => void; className?: string }) {
  const message = error instanceof Error ? error.message : 'Неизвестная ошибка';
  const code = error instanceof ApiError ? error.code : undefined;
  return (
    <div className={cn('flex flex-col items-center justify-center gap-2 px-6 py-10 text-center', className)}>
      <p className="flex items-center gap-2 font-medium">
        <i aria-hidden className="size-[7px] bg-danger" />
        Не удалось загрузить данные
      </p>
      <p className="max-w-md text-xs text-muted">
        {message}
        {code && <span className="ml-1 font-mono text-muted">[{code}]</span>}
      </p>
      {onRetry && (
        <Button variant="ghost" size="sm" onClick={onRetry}>
          Повторить
        </Button>
      )}
    </div>
  );
}

const BAR_TONES: Record<Tone | 'live', string> = {
  live: 'bg-accent',
  ok: 'bg-ok',
  warn: 'bg-warn',
  danger: 'bg-danger',
  info: 'bg-info',
  muted: 'bg-muted',
};

/** Flat progress bar; the accent ("live") while something is running. */
export function Progress({ value, tone = 'live', className }: { value: number; tone?: Tone | 'live'; className?: string }) {
  const pct = Math.max(0, Math.min(1, value)) * 100;
  return (
    <div className={cn('h-[3px] w-full bg-line', className)}>
      <div className={cn('h-full transition-[width] duration-300', BAR_TONES[tone])} style={{ width: `${pct}%` }} />
    </div>
  );
}

export interface NoteProps {
  tone?: 'info' | 'warn' | 'danger' | 'ok';
  title?: ReactNode;
  children?: ReactNode;
  className?: string;
}

const NOTE_SQUARE = { info: 'bg-info', warn: 'bg-warn', danger: 'bg-danger', ok: 'bg-ok' };

/** One quiet line (or short paragraph) of context: no box, no fill. Use sparingly. */
export function Note({ tone = 'info', title, children, className }: NoteProps) {
  return (
    <div className={cn('flex gap-2 text-xs text-muted', className)}>
      <i aria-hidden className={cn('mt-[6px] size-[7px] shrink-0', NOTE_SQUARE[tone])} />
      <div className="min-w-0">
        {title && <p className="font-medium text-fg">{title}</p>}
        {children && <div>{children}</div>}
      </div>
    </div>
  );
}
