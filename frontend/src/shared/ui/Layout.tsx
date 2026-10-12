import type { ReactNode } from 'react';
import { cn } from '../lib/cn';

export interface PageHeaderProps {
  title: ReactNode;
  /** One short line. Explanations belong in tooltips or the help overlay. */
  description?: ReactNode;
  actions?: ReactNode;
  meta?: ReactNode;
  className?: string;
}

export function PageHeader({ title, description, actions, meta, className }: PageHeaderProps) {
  return (
    <div className={cn('flex flex-wrap items-start justify-between gap-4 pb-4', className)}>
      <div className="min-w-0">
        <h1 className="text-[15px] leading-6 font-semibold">{title}</h1>
        {description && <p className="mt-0.5 max-w-3xl text-xs text-muted">{description}</p>}
        {meta && <div className="mt-1.5 flex flex-wrap items-center gap-3">{meta}</div>}
      </div>
      {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
    </div>
  );
}

/** Scrollable page container used by regular (non-workspace) pages. */
export function Page({ className, children }: { className?: string; children: ReactNode }) {
  return <div className={cn('mx-auto w-full max-w-[1400px] px-4 py-4', className)}>{children}</div>;
}

/** Flat pane separated by 1px lines: no radius, no shadow. */
export function Pane({ className, children }: { className?: string; children: ReactNode }) {
  return <section className={cn('border border-line bg-surface', className)}>{children}</section>;
}

export interface PaneHeaderProps {
  title: ReactNode;
  actions?: ReactNode;
  className?: string;
}

export function PaneHeader({ title, actions, className }: PaneHeaderProps) {
  return (
    <div className={cn('flex h-8 items-center justify-between gap-3 border-b border-line px-3', className)}>
      <h3 className="text-[13px] font-semibold">{title}</h3>
      {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
    </div>
  );
}

export interface ReadoutItem {
  label: ReactNode;
  value: ReactNode;
}

/** Compact measurement row: "label value" pairs in mono, like a scope's readout. */
export function Readout({ items, className }: { items: readonly ReadoutItem[]; className?: string }) {
  return (
    <dl className={cn('flex flex-wrap items-baseline gap-x-5 gap-y-1 font-mono text-xs', className)}>
      {items.map((item, i) => (
        <div key={i} className="flex items-baseline gap-1.5">
          <dt className="text-muted">{item.label}</dt>
          <dd>{item.value}</dd>
        </div>
      ))}
    </dl>
  );
}

export function KeyValue({ items, className }: { items: [ReactNode, ReactNode][]; className?: string }) {
  return (
    <dl className={cn('grid grid-cols-[minmax(110px,auto)_1fr] gap-x-4 gap-y-1 text-[13px]', className)}>
      {items.map(([k, v], i) => (
        <div key={i} className="contents">
          <dt className="text-muted">{k}</dt>
          <dd className="min-w-0 break-words">{v}</dd>
        </div>
      ))}
    </dl>
  );
}

/** Section label: sentence case, muted, never tracked or upper-cased. */
export function SectionTitle({ children, actions }: { children: ReactNode; actions?: ReactNode }) {
  return (
    <div className="mb-1.5 flex items-center justify-between">
      <h4 className="text-xs text-muted">{children}</h4>
      {actions}
    </div>
  );
}

export function Mono({ className, children, title }: { className?: string; children: ReactNode; title?: string }) {
  return (
    <code title={title} className={cn('font-mono text-xs', className)}>
      {children}
    </code>
  );
}
