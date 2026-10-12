import type { ReactNode } from 'react';
import { cn } from '../lib/cn';

export interface SegmentedItem<T extends string> {
  id: T;
  label: ReactNode;
  title?: string;
}

export interface SegmentedProps<T extends string> {
  items: readonly SegmentedItem<T>[];
  value: T;
  onChange: (value: T) => void;
  className?: string;
  'aria-label'?: string;
}

/** Exclusive choice as a row of keys: the active one is raised, the others stay quiet. */
export function Segmented<T extends string>({ items, value, onChange, className, 'aria-label': ariaLabel }: SegmentedProps<T>) {
  return (
    <div role="radiogroup" aria-label={ariaLabel} className={cn('inline-flex rounded-ctl border border-line-strong', className)}>
      {items.map((item) => {
        const active = item.id === value;
        return (
          <button
            key={item.id}
            type="button"
            role="radio"
            aria-checked={active}
            title={item.title}
            onClick={() => onChange(item.id)}
            className={cn(
              'inline-flex h-[26px] items-center gap-1.5 border-l border-line px-2.5 text-[13px] whitespace-nowrap transition-colors first:rounded-l-[2px] first:border-l-0 last:rounded-r-[2px]',
              active ? 'bg-surface-2 text-fg' : 'text-muted hover:text-fg',
            )}
          >
            {item.label}
          </button>
        );
      })}
    </div>
  );
}
