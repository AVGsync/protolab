import { CaretDownIcon } from '@phosphor-icons/react';
import type { ComponentProps, InputHTMLAttributes, ReactNode, TextareaHTMLAttributes } from 'react';
import { cn } from '../lib/cn';

const control =
  'rounded-ctl border border-line-strong bg-bg px-2 text-[13px] text-fg placeholder:text-subtle ' +
  'transition-colors hover:border-muted disabled:opacity-50';

export interface FieldProps {
  label: ReactNode;
  hint?: ReactNode;
  error?: ReactNode;
  className?: string;
  children: ReactNode;
}

export function Field({ label, hint, error, className, children }: FieldProps) {
  return (
    <label className={cn('flex flex-col gap-1', className)}>
      <span className="text-xs text-muted">{label}</span>
      {children}
      {error ? <span className="text-xs text-danger">{error}</span> : hint && <span className="text-xs text-subtle">{hint}</span>}
    </label>
  );
}

export function Input({ className, ...rest }: InputHTMLAttributes<HTMLInputElement>) {
  return <input className={cn(control, 'h-7', className)} {...rest} />;
}

export function Textarea({ className, ...rest }: TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return <textarea className={cn(control, 'min-h-20 py-1.5 leading-5', className)} {...rest} />;
}

/** `className` sizes the whole control (e.g. `w-64`); other props go to the native select. */
export function Select({ className, children, ...rest }: ComponentProps<'select'>) {
  return (
    <span className={cn('relative inline-flex min-w-0', className)}>
      <select className={cn(control, 'h-7 w-full min-w-0 appearance-none pr-6')} {...rest}>
        {children}
      </select>
      <CaretDownIcon size={12} aria-hidden className="pointer-events-none absolute top-1/2 right-2 -translate-y-1/2 text-muted" />
    </span>
  );
}

export interface CheckboxProps extends Omit<InputHTMLAttributes<HTMLInputElement>, 'type'> {
  label: ReactNode;
}

export function Checkbox({ label, className, ...rest }: CheckboxProps) {
  return (
    <label className={cn('inline-flex cursor-pointer items-center gap-2 text-[13px] select-none', className)}>
      <input type="checkbox" className="size-3.5 accent-fg" {...rest} />
      <span>{label}</span>
    </label>
  );
}
