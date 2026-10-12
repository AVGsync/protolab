import { AlertTriangle, Inbox, Loader2, RotateCw } from 'lucide-react';
import type { ReactNode } from 'react';
import { ApiError } from '../api/http';
import { cn } from '../lib/cn';
import type { Tone } from './Badge';

export function Spinner({ size = 16, className }: { size?: number; className?: string }) {
  return <Loader2 size={size} className={cn('animate-spin', className)} aria-hidden />;
}

export function Loader({ label = 'Загрузка…', className }: { label?: string; className?: string }) {
  return (
    <div className={cn('flex items-center justify-center gap-2 py-10 text-muted', className)}>
      <Spinner />
      <span>{label}</span>
    </div>
  );
}

export interface EmptyStateProps {
  icon?: ReactNode;
  title: ReactNode;
  description?: ReactNode;
  action?: ReactNode;
  className?: string;
}

export function EmptyState({ icon, title, description, action, className }: EmptyStateProps) {
  return (
    <div className={cn('flex flex-col items-center justify-center px-6 py-12 text-center', className)}>
      <div className="mb-3 rounded-full border border-line bg-surface-2 p-3 text-muted">{icon ?? <Inbox size={20} />}</div>
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
      <AlertTriangle className="text-danger" size={22} />
      <p className="font-medium">Не удалось загрузить данные</p>
      <p className="max-w-md text-xs text-muted">
        {message}
        {code && <span className="ml-1 font-mono text-subtle">[{code}]</span>}
      </p>
      {onRetry && (
        <button
          type="button"
          onClick={onRetry}
          className="mt-2 inline-flex items-center gap-1.5 text-xs text-info hover:underline"
        >
          <RotateCw size={12} /> Повторить
        </button>
      )}
    </div>
  );
}

const BAR_TONES: Partial<Record<Tone, string>> = {
  accent: 'bg-accent',
  success: 'bg-success',
  warning: 'bg-warning',
  danger: 'bg-danger',
  info: 'bg-info',
  neutral: 'bg-muted',
};

export function Progress({ value, tone = 'accent', className }: { value: number; tone?: Tone; className?: string }) {
  const pct = Math.max(0, Math.min(1, value)) * 100;
  return (
    <div className={cn('h-1.5 w-full overflow-hidden rounded-full bg-surface-3', className)}>
      <div className={cn('h-full rounded-full transition-[width] duration-300', BAR_TONES[tone])} style={{ width: `${pct}%` }} />
    </div>
  );
}

export interface CalloutProps {
  tone?: 'info' | 'warning' | 'danger' | 'success';
  icon?: ReactNode;
  title?: ReactNode;
  children?: ReactNode;
  className?: string;
}

const CALLOUT_TONES = {
  info: 'border-info/30 bg-info/8 text-info',
  warning: 'border-warning/30 bg-warning/8 text-warning',
  danger: 'border-danger/30 bg-danger/8 text-danger',
  success: 'border-success/30 bg-success/8 text-success',
};

export function Callout({ tone = 'info', icon, title, children, className }: CalloutProps) {
  return (
    <div className={cn('flex gap-2.5 rounded-md border px-3 py-2.5', CALLOUT_TONES[tone], className)}>
      {icon && <div className="mt-0.5 shrink-0">{icon}</div>}
      <div className="min-w-0 text-[12.5px]">
        {title && <p className="font-medium">{title}</p>}
        {children && <div className="text-fg/80">{children}</div>}
      </div>
    </div>
  );
}
