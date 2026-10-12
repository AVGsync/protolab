import { useQuery } from '@tanstack/react-query';
import { useState } from 'react';
import { Link } from 'react-router';
import { hypothesisQueries } from '@/entities/hypothesis';
import { observationQueries, SourceRefLink } from '@/entities/observation';
import { routes } from '@/shared/config';
import { formatDateTime, useRequiredParams } from '@/shared/lib';
import { EmptyState, ErrorState, Loader, Page, PageHeader, Pane, Select, Status, Table, TBody, Td, Th, THead, Tr } from '@/shared/ui';

export function ObservationsPage() {
  const { projectId } = useRequiredParams('projectId');
  const [hypothesisId, setHypothesisId] = useState('');
  const observations = useQuery(observationQueries.list(projectId, { hypothesisId: hypothesisId || undefined }));
  const hypotheses = useQuery(hypothesisQueries.list(projectId));
  const titleById = new Map(hypotheses.data?.items.map((h) => [h.id, h.title]));

  return (
    <Page>
      <PageHeader
        title="Наблюдения"
        actions={
          <Select value={hypothesisId} onChange={(e) => setHypothesisId(e.target.value)} className="w-72">
            <option value="">Все гипотезы</option>
            {hypotheses.data?.items.map((h) => (
              <option key={h.id} value={h.id}>
                {h.id}: {h.title}
              </option>
            ))}
          </Select>
        }
      />
      {observations.isLoading && <Loader />}
      {observations.error && <ErrorState error={observations.error} onRetry={observations.refetch} />}
      {observations.data && observations.data.items.length === 0 && (
        <EmptyState
          title="Наблюдений нет"
          description="Выделите байты в потоке сессии и нажмите «Наблюдение»."
        />
      )}
      {observations.data && observations.data.items.length > 0 && (
        <Pane>
          <Table>
            <THead>
              <tr>
                <Th>Наблюдение</Th>
                <Th>Источник</Th>
                <Th>Снимок байтов</Th>
                <Th>Гипотезы</Th>
                <Th>Создано</Th>
              </tr>
            </THead>
            <TBody>
              {observations.data.items.map((o) => (
                <Tr key={o.id} className="align-top">
                  <Td className="max-w-sm">{o.note}</Td>
                  <Td>
                    <SourceRefLink projectId={projectId} source={o.source} />
                    <div className="mt-0.5 font-mono text-[10.5px] text-subtle">пакеты #{o.source.packetNumbers.join(', #')}</div>
                  </Td>
                  <Td>
                    <code className="font-mono text-[11.5px] text-fg">{o.snapshotHex}</code>
                    {o.integrity === 'mismatch' && (
                      <div className="mt-1">
                        <Status tone="danger">не совпадает с источником</Status>
                      </div>
                    )}
                  </Td>
                  <Td>
                    <div className="flex flex-col gap-0.5">
                      {o.hypothesisIds.map((id) => (
                        <Link key={id} to={routes.hypotheses(projectId, id)} className="text-[12px] link">
                          {id}: {titleById.get(id) ?? ''}
                        </Link>
                      ))}
                    </div>
                  </Td>
                  <Td className="text-[12px] whitespace-nowrap text-muted">{formatDateTime(o.createdAt)}</Td>
                </Tr>
              ))}
            </TBody>
          </Table>
        </Pane>
      )}
    </Page>
  );
}
