import type { HTMLAttributes, TdHTMLAttributes, ThHTMLAttributes } from 'react';
import { cn } from '../lib/cn';

/** Square table, 28px rows, mono numbers are right-aligned by the caller (`text-right font-mono`). */
export function Table({ className, ...rest }: HTMLAttributes<HTMLTableElement>) {
  return (
    <div className="w-full overflow-x-auto">
      <table className={cn('w-full border-collapse text-left text-sm', className)} {...rest} />
    </div>
  );
}

export function THead({ className, ...rest }: HTMLAttributes<HTMLTableSectionElement>) {
  return <thead className={cn('sticky top-0 z-10 bg-surface', className)} {...rest} />;
}

export function TBody(props: HTMLAttributes<HTMLTableSectionElement>) {
  return <tbody {...props} />;
}

export interface TrProps extends HTMLAttributes<HTMLTableRowElement> {
  selected?: boolean;
}

export function Tr({ className, selected, onClick, ...rest }: TrProps) {
  return (
    <tr
      onClick={onClick}
      className={cn(
        'border-b border-line last:border-b-0',
        onClick && 'cursor-pointer hover:bg-surface-2',
        selected && 'bg-selection hover:bg-selection',
        className,
      )}
      {...rest}
    />
  );
}

export function Th({ className, ...rest }: ThHTMLAttributes<HTMLTableCellElement>) {
  return <th className={cn('legend h-7 border-b border-line px-3 text-xs font-normal whitespace-nowrap text-muted', className)} {...rest} />;
}

export function Td({ className, ...rest }: TdHTMLAttributes<HTMLTableCellElement>) {
  return <td className={cn('h-7 px-3 py-0.5 align-middle', className)} {...rest} />;
}
