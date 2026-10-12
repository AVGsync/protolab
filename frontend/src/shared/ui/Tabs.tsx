import type { ReactNode } from 'react';
import { cn } from '../lib/cn';

export interface TabItem<T extends string> {
  id: T;
  label: ReactNode;
  badge?: ReactNode;
  icon?: ReactNode;
}

export interface TabsProps<T extends string> {
  items: readonly TabItem<T>[];
  value: T;
  onChange: (value: T) => void;
  className?: string;
}

export function Tabs<T extends string>({ items, value, onChange, className }: TabsProps<T>) {
  return (
    <div role="tablist" className={cn('flex items-center gap-1 border-b border-line px-2', className)}>
      {items.map((item) => {
        const active = item.id === value;
        return (
          <button
            key={item.id}
            type="button"
            role="tab"
            aria-selected={active}
            onClick={() => onChange(item.id)}
            className={cn(
              '-mb-px inline-flex h-9 items-center gap-1.5 border-b-2 px-2.5 text-[12.5px] font-medium transition-colors',
              active ? 'border-accent text-fg' : 'border-transparent text-muted hover:text-fg',
            )}
          >
            {item.icon}
            {item.label}
            {item.badge !== undefined && (
              <span className="rounded bg-surface-3 px-1.5 text-[10.5px] text-muted">{item.badge}</span>
            )}
          </button>
        );
      })}
    </div>
  );
}
