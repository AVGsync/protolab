import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router';
import { ACTION_KIND_LABEL, ActionParams, actionQueries } from '@/entities/action';
import { ImportActionsButton } from '@/features/import-actions';
import { routes } from '@/shared/config';
import { formatDuration, formatTimestamp, useRequiredParams } from '@/shared/lib';
import { EmptyState, ErrorState, Loader, Note, Page, PageHeader, Pane, Status, Table, TBody, Td, Th, THead, Tr } from '@/shared/ui';

export function ActionsPage() {
  const { projectId } = useRequiredParams('projectId');
  const { data, isLoading, error, refetch } = useQuery(actionQueries.list(projectId));

  return (
    <Page>
      <PageHeader
        title="Журнал действий"
        actions={<ImportActionsButton projectId={projectId} />}
      />
      <Note tone="info" className="mb-4">
        Формат импорта, CSV: <code className="font-mono text-[11.5px]">timestamp,kind,label,params,result</code>, где params —{' '}
        <code className="font-mono text-[11.5px]">param=setpoint;value=21</code> или JSON. Также принимается JSON-массив действий.
      </Note>
      {isLoading && <Loader />}
      {error && <ErrorState error={error} onRetry={refetch} />}
      {data && data.items.length === 0 && <EmptyState title="Журнал пуст" description="Импортируйте журнал действий из CSV или JSON." />}
      {data && data.items.length > 0 && (
        <Pane>
          <Table>
            <THead>
              <tr>
                <Th>Время (UTC)</Th>
                <Th>Действие</Th>
                <Th>Параметры</Th>
                <Th>Наблюдаемый результат</Th>
                <Th>Сессии</Th>
              </tr>
            </THead>
            <TBody>
              {data.items.map((a) => (
                <Tr key={a.id} className="align-top">
                  <Td className="font-mono text-[12px] whitespace-nowrap text-muted">
                    {a.timestamp.slice(0, 10)}
                    <br />
                    {formatTimestamp(a.timestamp, 3)}
                  </Td>
                  <Td>
                    <p className="font-medium">{a.label}</p>
                    <p className="text-[11px] text-subtle">
                      {ACTION_KIND_LABEL[a.kind]} · {a.source === 'import' ? 'импорт' : 'вручную'}
                    </p>
                  </Td>
                  <Td>
                    <ActionParams params={a.params} />
                  </Td>
                  <Td className="text-[12.5px]">
                    {a.result ?? '—'}
                    {a.observed && (
                      <div className="mt-1">
                        <ActionParams params={a.observed} />
                      </div>
                    )}
                  </Td>
                  <Td>
                    {a.links.length === 0 ? (
                      <Status tone="warn">не связано</Status>
                    ) : (
                      a.links.map((l) => (
                        <Link
                          key={l.sessionId}
                          to={routes.session(projectId, l.sessionId)}
                          className="link flex items-center gap-2 font-mono text-xs"
                        >
                          {l.sessionId}
                          <span className="text-[11px] text-subtle">
                            {l.method === 'auto_time' ? `по времени, сдвиг ${formatDuration(l.deltaMs ?? 0)}` : 'вручную'}
                          </span>
                        </Link>
                      ))
                    )}
                  </Td>
                </Tr>
              ))}
            </TBody>
          </Table>
        </Pane>
      )}
    </Page>
  );
}
