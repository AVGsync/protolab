import type { ReactNode } from 'react';
import { NavLink } from 'react-router';
import { cn } from '../lib/cn';

export interface TabItem<T extends string> {
  id: T;
  label: ReactNode;
  /** Quiet count after the label. */
  count?: ReactNode;
}

const tabBase = '-mb-px inline-flex h-8 items-center gap-1.5 border-b-2 px-2.5 text-sm whitespace-nowrap transition-colors';
const tabClass = (active: boolean) =>
  cn(tabBase, active ? 'border-fg font-medium text-fg' : 'border-transparent text-muted hover:text-fg');

function Count({ children }: { children: ReactNode }) {
  return <span className="font-mono text-2xs font-normal text-muted">{children}</span>;
}

export interface TabsProps<T extends string> {
  items: readonly TabItem<T>[];
  value: T;
  onChange: (value: T) => void;
  className?: string;
}

/** In-page tabs (state). For tabs that change the URL use TabNav. */
export function Tabs<T extends string>({ items, value, onChange, className }: TabsProps<T>) {
  return (
    <div role="tablist" className={cn('flex items-center gap-1 border-b border-line px-2', className)}>
      {items.map((item) => (
        <button
          key={item.id}
          type="button"
          role="tab"
          aria-selected={item.id === value}
          onClick={() => onChange(item.id)}
          className={tabClass(item.id === value)}
        >
          {item.label}
          {item.count !== undefined && <Count>{item.count}</Count>}
        </button>
      ))}
    </div>
  );
}

export interface TabNavItem {
  to: string;
  label: ReactNode;
  count?: ReactNode;
  /** Match the path exactly (for an index tab). */
  end?: boolean;
}

/** Tabs that are routes (sub-sections of a project section). */
export function TabNav({ items, className }: { items: readonly TabNavItem[]; className?: string }) {
  return (
    <nav className={cn('flex items-center gap-1 border-b border-line px-2', className)}>
      {items.map((item) => (
        <NavLink key={item.to} to={item.to} end={item.end} className={({ isActive }) => tabClass(isActive)}>
          {item.label}
          {item.count !== undefined && <Count>{item.count}</Count>}
        </NavLink>
      ))}
    </nav>
  );
}
