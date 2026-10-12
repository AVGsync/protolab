import { Check, Copy } from 'lucide-react';
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
      className={cn('inline-flex items-center gap-1 rounded px-1 text-subtle hover:bg-surface-3 hover:text-fg', className)}
    >
      {copied ? <Check size={12} className="text-success" /> : <Copy size={12} />}
      {label && <span className="text-xs">{label}</span>}
    </button>
  );
}
