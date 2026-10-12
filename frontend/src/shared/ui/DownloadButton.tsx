import { useState, type ReactNode } from 'react';
import { downloadFromUrl } from '../lib/download';
import { buttonClass, type ButtonSize, type ButtonVariant } from './button-styles';
import { Spinner } from './Feedback';
import { toast } from './toast-store';

export interface DownloadButtonProps {
  href: string;
  fileName: string;
  children: ReactNode;
  variant?: ButtonVariant;
  size?: ButtonSize;
  /** Replaces the default button look (e.g. for card-like download tiles). */
  className?: string;
}

export function DownloadButton({ href, fileName, children, variant = 'secondary', size = 'md', className }: DownloadButtonProps) {
  const [busy, setBusy] = useState(false);
  const download = async () => {
    setBusy(true);
    try {
      await downloadFromUrl(href, fileName);
    } catch (err) {
      toast.fromError(err, 'Не удалось скачать файл');
    } finally {
      setBusy(false);
    }
  };
  return (
    <button type="button" onClick={download} disabled={busy} className={className ?? buttonClass(variant, size)}>
      {busy && !className && <Spinner size={14} />}
      {children}
    </button>
  );
}
