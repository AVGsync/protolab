import type { ReactNode } from 'react';
import { cn, ENCODING_SIZE, type CandidateSearch, type FieldCandidate } from '@/shared/lib';
import { Note, Status } from '@/shared/ui';

export interface CandidateListProps {
  search: CandidateSearch;
  rowLabels: string[];
  active?: FieldCandidate | null;
  onPick: (candidate: FieldCandidate) => void;
  renderAction?: (candidate: FieldCandidate) => ReactNode;
}

/** Explainable suggestions: every candidate states its basis and its counterexamples. */
export function CandidateList({ search, rowLabels, active, onPick, renderAction }: CandidateListProps) {
  if (search.warning) return <Note tone="info">{search.warning}</Note>;
  if (search.candidates.length === 0) {
    return <p className="text-xs text-muted">Ни одна позиция не совпадает со значением параметра хотя бы в половине примеров.</p>;
  }
  return (
    <ul className="flex flex-col gap-1.5">
      {search.candidates.slice(0, 12).map((c) => {
        const isActive = active && active.offset === c.offset && active.encoding === c.encoding && active.scale === c.scale;
        return (
          <li
            key={`${c.offset}:${c.encoding}:${c.scale}`}
            className={cn('border px-3 py-2', isActive ? 'border-line-strong bg-selection' : 'border-line bg-surface')}
          >
            <div className="flex items-center gap-2">
              <button type="button" onClick={() => onPick(c)} className="flex flex-1 items-center gap-2 text-left">
                <span className="font-mono text-[12.5px]">
                  @{c.offset}
                  {ENCODING_SIZE[c.encoding] > 1 && `–${c.offset + ENCODING_SIZE[c.encoding] - 1}`} {c.encoding}
                  {c.scale !== 1 && ` ÷${c.scale}`}
                </span>
                <Status tone={c.matches === c.total ? 'ok' : 'warn'} className="font-mono text-xs">
                  {c.matches}/{c.total}
                </Status>
              </button>
              {renderAction?.(c)}
            </div>
            <p className="mt-1 text-[11.5px] text-muted">
              {c.counterexamples.length === 0
                ? 'Совпадает со значением параметра во всех примерах — проверьте на дополнительной записи.'
                : `Контрпримеры: ${c.counterexamples.map((i) => rowLabels[i]).join('; ')}`}
            </p>
          </li>
        );
      })}
    </ul>
  );
}
