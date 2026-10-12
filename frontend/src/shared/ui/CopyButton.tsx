import { CheckIcon, CopyIcon } from '@phosphor-icons/react';
import { useState } from 'react';
import { cn } from '../lib/cn';
import { toast } from './toast-store';

export function CopyButton({ value, label, className }: { value: string; label?: string; className?: string }) {
  const [copied, setCopied] = useState(false);

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      setTimeout(() => setCopied(false), 1200);
    } catch {
      toast.error('Не удалось скопировать в буфер обмена');
    }
  };

  return (
    <button
      type="button"
      onClick={copy}
      title="Скопировать"
      className={cn('inline-flex items-center gap-1 rounded-ctl px-1 text-subtle hover:bg-surface-2 hover:text-fg', className)}
    >
      {copied ? <CheckIcon size={12} className="text-ok" /> : <CopyIcon size={12} />}
      {label && <span className="text-xs">{label}</span>}
    </button>
  );
}
