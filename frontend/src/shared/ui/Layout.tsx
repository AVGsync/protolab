import type { ReactNode } from 'react';
import { cn } from '../lib/cn';
import type { Tone } from './Badge';

export interface PageHeaderProps {
  title: ReactNode;
  description?: ReactNode;
  actions?: ReactNode;
  meta?: ReactNode;
  className?: string;
}

export function PageHeader({ title, description, actions, meta, className }: PageHeaderProps) {
  return (
    <div className={cn('flex flex-wrap items-start justify-between gap-4 pb-5', className)}>
      <div className="min-w-0">
        <h1 className="text-lg font-semibold tracking-tight">{title}</h1>
        {description && <p className="mt-1 max-w-3xl text-[12.5px] text-muted">{description}</p>}
        {meta && <div className="mt-2 flex flex-wrap items-center gap-2">{meta}</div>}
      </div>
      {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
    </div>
  );
}

/** Scrollable page container used by regular (non-workspace) pages. */
export function Page({ className, children }: { className?: string; children: ReactNode }) {
  return <div className={cn('mx-auto w-full max-w-[1400px] px-6 py-6', className)}>{children}</div>;
}

const STAT_TONES: Partial<Record<Tone, string>> = {
  success: 'text-success',
  warning: 'text-warning',
  danger: 'text-danger',
  info: 'text-info',
  accent: 'text-[#ff6b8f]',
  violet: 'text-violet',
};

export interface StatProps {
  label: ReactNode;
  value: ReactNode;
  hint?: ReactNode;
  icon?: ReactNode;
  tone?: Tone;
  className?: string;
}

export function Stat({ label, value, hint, icon, tone, className }: StatProps) {
  return (
    <div className={cn('rounded-lg border border-line bg-surface px-4 py-3', className)}>
      <div className="flex items-center justify-between text-xs text-muted">
        <span>{label}</span>
        {icon}
      </div>
      <div className={cn('mt-1 text-xl font-semibold tabular-nums', tone && STAT_TONES[tone])}>{value}</div>
      {hint && <div className="mt-0.5 text-[11.5px] text-subtle">{hint}</div>}
    </div>
  );
}

export function KeyValue({ items, className }: { items: [ReactNode, ReactNode][]; className?: string }) {
  return (
    <dl className={cn('grid grid-cols-[minmax(110px,auto)_1fr] gap-x-4 gap-y-1.5 text-[12.5px]', className)}>
      {items.map(([k, v], i) => (
        <div key={i} className="contents">
          <dt className="text-muted">{k}</dt>
          <dd className="min-w-0 break-words">{v}</dd>
        </div>
      ))}
    </dl>
  );
}

export function SectionTitle({ children, actions }: { children: ReactNode; actions?: ReactNode }) {
  return (
    <div className="mb-2 flex items-center justify-between">
      <h4 className="text-[11px] font-semibold tracking-wider text-subtle uppercase">{children}</h4>
      {actions}
    </div>
  );
}

export function Mono({ className, children, title }: { className?: string; children: ReactNode; title?: string }) {
  return (
    <code title={title} className={cn('font-mono text-[12px]', className)}>
      {children}
    </code>
  );
}
