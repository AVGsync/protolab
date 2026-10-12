import { cn } from '../lib/cn';

export type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger';
export type ButtonSize = 'xs' | 'sm' | 'md';

const VARIANTS: Record<ButtonVariant, string> = {
  // The primary action is a neutral "hard key": inverted text colours. Crimson is never a fill.
  primary: 'border-fg bg-fg text-bg hover:bg-fg/85',
  secondary: 'border-line-strong bg-transparent text-fg hover:bg-surface-2',
  ghost: 'border-transparent bg-transparent text-muted hover:bg-surface-2 hover:text-fg',
  danger: 'border-danger/60 bg-transparent text-danger hover:bg-danger/10',
};

const SIZES: Record<ButtonSize, string> = {
  xs: 'h-5 gap-1 px-1.5 text-xs',
  sm: 'h-6 gap-1.5 px-2 text-xs',
  md: 'h-7 gap-2 px-3 text-[13px]',
};

/** Shared button look, also used for router links styled as buttons. */
export function buttonClass(variant: ButtonVariant = 'secondary', size: ButtonSize = 'md', className?: string): string {
  return cn(
    'inline-flex shrink-0 items-center justify-center rounded-ctl border font-medium whitespace-nowrap transition-colors select-none',
    'disabled:pointer-events-none disabled:opacity-45',
    VARIANTS[variant],
    SIZES[size],
    className,
  );
}
