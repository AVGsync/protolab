import type { ReactNode } from 'react';
import { SourceRefLink } from '@/entities/observation';
import { FindingKindMark, type Finding } from '@/entities/verification';
import type { SourceRef } from '@/shared/api';
import { formatNumber } from '@/shared/lib';
import { EmptyState, Table, TBody, Td, Th, THead, Tr } from '@/shared/ui';

export interface FindingsTableProps {
  projectId: string;
  findings: Finding[];
  /** Version used for framing when jumping to the source. */
  versionId: string;
  /** Title of the empty state. */
  empty?: string;
  /** An action next to an example, e.g. recording it as a counterexample. */
  exampleAction?: (finding: Finding, example: SourceRef) => ReactNode;
}

export function FindingsTable({ projectId, findings, versionId, empty = 'Нет находок этого типа', exampleAction }: FindingsTableProps) {
  if (findings.length === 0) return <EmptyState title={empty} />;
  return (
    <Table>
      <THead>
        <tr>
          <Th>Тип</Th>
          <Th>Описание</Th>
          <Th>Правило / поле</Th>
          <Th className="text-right">Случаев</Th>
          <Th>Примеры (переход к байтам)</Th>
        </tr>
      </THead>
      <TBody>
        {findings.map((f) => (
          <Tr key={f.id} className="align-top">
            <Td className="py-1.5">
              <FindingKindMark kind={f.kind} />
            </Td>
            <Td className="max-w-md min-w-56 py-1.5">{f.message}</Td>
            <Td className="py-1.5 font-mono text-2xs text-muted">
              {f.ruleId ?? '—'}
              {f.fieldPath && <span className="text-fg">.{f.fieldPath}</span>}
            </Td>
            <Td className="py-1.5 text-right font-mono">{formatNumber(f.occurrences)}</Td>
            <Td className="py-1.5">
              <div className="flex flex-col gap-0.5">
                {f.examples.slice(0, 3).map((ex, i) => (
                  <span key={i} className="flex items-center gap-2">
                    <SourceRefLink projectId={projectId} source={ex} versionId={versionId} />
                    {exampleAction?.(f, ex)}
                  </span>
                ))}
                {f.examples.length > 3 && <span className="text-2xs text-muted">ещё {f.examples.length - 3}…</span>}
              </div>
            </Td>
          </Tr>
        ))}
      </TBody>
    </Table>
  );
}
