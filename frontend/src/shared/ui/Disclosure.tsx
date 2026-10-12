import { CaretRightIcon } from '@phosphor-icons/react';
import type { ReactNode } from 'react';
import { cn } from '../lib/cn';

export interface DisclosureProps {
  /** The always visible line. */
  summary: ReactNode;
  children: ReactNode;
  defaultOpen?: boolean;
  className?: string;
}

/**
 * Progressive disclosure: secondary detail stays one quiet line until asked for.
 * A native <details>, so it works with the keyboard and without any state.
 */
export function Disclosure({ summary, children, defaultOpen, className }: DisclosureProps) {
  return (
    <details open={defaultOpen} className={cn('group', className)}>
      <summary className="inline-flex cursor-pointer list-none items-center gap-1 text-xs text-muted select-none hover:text-fg [&::-webkit-details-marker]:hidden">
        <CaretRightIcon size={12} aria-hidden className="transition-transform group-open:rotate-90" />
        {summary}
      </summary>
      <div className="mt-2">{children}</div>
    </details>
  );
}
