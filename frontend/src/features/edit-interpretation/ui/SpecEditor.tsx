import { AlertTriangle, CheckCircle2, XCircle } from 'lucide-react';
import { useRef } from 'react';
import type { SpecIssue } from '@/entities/interpretation';
import { cn } from '@/shared/lib';

export interface SpecEditorProps {
  value: string;
  onChange: (value: string) => void;
  syntaxError: string | null;
  issues?: SpecIssue[];
  readOnly?: boolean;
  className?: string;
}

/** JSON editor for "protolab/v1" specs with line numbers and validation output. */
export function SpecEditor({ value, onChange, syntaxError, issues = [], readOnly, className }: SpecEditorProps) {
  const gutter = useRef<HTMLDivElement>(null);
  const lines = value.split('\n').length;
  const errors = issues.filter((i) => i.severity === 'error');
  const warnings = issues.filter((i) => i.severity === 'warning');

  return (
    <div className={cn('flex min-h-0 flex-col', className)}>
      <div className="relative flex min-h-0 flex-1 overflow-hidden rounded-t-md border border-line bg-bg">
        <div
          ref={gutter}
          aria-hidden
          className="w-11 shrink-0 overflow-hidden border-r border-line bg-surface py-2 pr-2 text-right font-mono text-[11.5px] leading-[19px] text-subtle select-none"
        >
          {Array.from({ length: lines }, (_, i) => (
            <div key={i}>{i + 1}</div>
          ))}
        </div>
        <textarea
          value={value}
          readOnly={readOnly}
          spellCheck={false}
          onChange={(e) => onChange(e.target.value)}
          onScroll={(e) => {
            if (gutter.current) gutter.current.scrollTop = e.currentTarget.scrollTop;
          }}
          onKeyDown={(e) => {
            if (e.key !== 'Tab' || readOnly) return;
            e.preventDefault();
            const el = e.currentTarget;
            const { selectionStart: s, selectionEnd: end } = el;
            const next = `${value.slice(0, s)}  ${value.slice(end)}`;
            onChange(next);
            requestAnimationFrame(() => el.setSelectionRange(s + 2, s + 2));
          }}
          className="min-h-0 flex-1 resize-none bg-transparent px-3 py-2 font-mono text-[12px] leading-[19px] whitespace-pre text-fg outline-none"
        />
      </div>
      <div className="max-h-36 overflow-y-auto rounded-b-md border border-t-0 border-line bg-surface px-3 py-2 text-[12px]">
        {syntaxError ? (
          <p className="flex items-center gap-2 text-danger">
            <XCircle size={13} /> Синтаксис JSON: {syntaxError}
          </p>
        ) : errors.length === 0 && warnings.length === 0 ? (
          <p className="flex items-center gap-2 text-success">
            <CheckCircle2 size={13} /> JSON корректен
          </p>
        ) : (
          <ul className="flex flex-col gap-1">
            {[...errors, ...warnings].map((issue, i) => (
              <li key={i} className={cn('flex items-start gap-2', issue.severity === 'error' ? 'text-danger' : 'text-warning')}>
                {issue.severity === 'error' ? <XCircle size={13} className="mt-0.5" /> : <AlertTriangle size={13} className="mt-0.5" />}
                <code className="font-mono text-[11px] text-muted">{issue.path}</code>
                <span className="text-fg/85">{issue.message}</span>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}
