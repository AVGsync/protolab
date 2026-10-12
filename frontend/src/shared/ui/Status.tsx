import type { ReactNode } from 'react';
import { cn } from '../lib/cn';

export type Tone = 'ok' | 'warn' | 'danger' | 'info' | 'muted';

const SQUARE: Record<Tone, string> = {
  ok: 'bg-ok',
  warn: 'bg-warn',
  danger: 'bg-danger',
  info: 'bg-info',
  muted: 'bg-muted',
};

export interface StatusProps {
  tone?: Tone;
  title?: string;
  className?: string;
  children: ReactNode;
}

/**
 * Attention or fault marker: a 7px square in the tone colour followed by plain text.
 * The normal state is silent, so render this only when something deserves a look.
 */
export function Status({ tone = 'muted', title, className, children }: StatusProps) {
  return (
    <span title={title} className={cn('inline-flex items-center gap-1.5 whitespace-nowrap', tone === 'muted' ? 'text-muted' : 'text-fg', className)}>
      <i aria-hidden className={cn('size-[7px] shrink-0', SQUARE[tone])} />
      {children}
    </span>
  );
}

/** Quiet mono tag such as [v3] or [устарела]. */
export function Tag({ title, className, children }: { title?: string; className?: string; children: ReactNode }) {
  return (
    <span title={title} className={cn('inline-flex items-center font-mono text-2xs leading-4 whitespace-nowrap text-muted', className)}>
      [{children}]
    </span>
  );
}

/** Keycap legend: the same keys exist in the TUI of the same binary. */
export function Keycap({ className, children }: { className?: string; children: ReactNode }) {
  return (
    <kbd
      className={cn(
        'inline-flex h-4 min-w-4 items-center justify-center rounded-ctl border border-b-2 border-line-strong px-1 font-mono text-2xs leading-none text-fg',
        className,
      )}
    >
      {children}
    </kbd>
  );
}

/** Arrows are drawn as SVG: the bundled font subsets have no arrow glyphs. */
export function Arrow({ dir = 'right', className }: { dir?: 'right' | 'left'; className?: string }) {
  return (
    <svg
      viewBox="0 0 12 8"
      width="12"
      height="8"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.3"
      aria-hidden
      className={cn('inline-block shrink-0', dir === 'left' && '-scale-x-100', className)}
    >
      <path d="M0 4h10.5M7.5 1l3 3-3 3" />
    </svg>
  );
}
