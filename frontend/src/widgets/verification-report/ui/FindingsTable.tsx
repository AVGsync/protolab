import { SourceRefLink } from '@/entities/observation';
import { FindingKindMark, type Finding } from '@/entities/verification';
import { formatNumber } from '@/shared/lib';
import { EmptyState, Table, TBody, Td, Th, THead, Tr } from '@/shared/ui';

export interface FindingsTableProps {
  projectId: string;
  findings: Finding[];
  /** Version used for framing when jumping to the source. */
  versionId: string;
}

export function FindingsTable({ projectId, findings, versionId }: FindingsTableProps) {
  if (findings.length === 0) return <EmptyState title="Нет находок этого типа" />;
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
            <Td>
              <FindingKindMark kind={f.kind} />
            </Td>
            <Td className="max-w-md">{f.message}</Td>
            <Td className="font-mono text-[11.5px] text-muted">
              {f.ruleId ?? '—'}
              {f.fieldPath && <span className="text-fg">.{f.fieldPath}</span>}
            </Td>
            <Td className="text-right font-mono">{formatNumber(f.occurrences)}</Td>
            <Td>
              <div className="flex flex-col gap-0.5">
                {f.examples.slice(0, 3).map((ex, i) => (
                  <SourceRefLink key={i} projectId={projectId} source={ex} versionId={versionId} />
                ))}
                {f.examples.length > 3 && <span className="text-[11px] text-subtle">ещё {f.examples.length - 3}…</span>}
              </div>
            </Td>
          </Tr>
        ))}
      </TBody>
    </Table>
  );
}
