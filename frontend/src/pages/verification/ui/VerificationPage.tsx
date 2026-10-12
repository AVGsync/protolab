import { useQuery } from '@tanstack/react-query';
import { useNavigate } from 'react-router';
import { corpusQueries, describeCorpusFilter } from '@/entities/corpus';
import { RunOutcome, RunStatusMark, StaleTag, verificationQueries } from '@/entities/verification';
import { CreateCorpusButton } from '@/features/create-corpus';
import { StartRunButton } from '@/features/run-verification';
import { routes } from '@/shared/config';
import { formatDateTime, formatDuration, shortHash, useRequiredParams } from '@/shared/lib';
import { EmptyState, ErrorState, Loader, Page, PageHeader, Pane, PaneHeader, Table, Tag, TBody, Td, Th, THead, Tr } from '@/shared/ui';

export function VerificationPage() {
  const { projectId } = useRequiredParams('projectId');
  const navigate = useNavigate();
  const corpora = useQuery(corpusQueries.list(projectId));
  const runs = useQuery({
    ...verificationQueries.runs(projectId),
    refetchInterval: (q) => (q.state.data?.items.some((r) => r.status === 'running' || r.status === 'queued') ? 1000 : false),
  });

  return (
    <Page>
      <PageHeader
        title="Проверка интерпретаций"
        actions={
          <>
            <CreateCorpusButton projectId={projectId} />
            <StartRunButton projectId={projectId} />
          </>
        }
      />

      <Pane className="mb-4">
        <PaneHeader title="Корпуса" />
        {corpora.isLoading && <Loader />}
        {corpora.data && corpora.data.items.length === 0 && <p className="px-3 py-3 text-xs text-muted">Создайте корпус для проверки.</p>}
        {corpora.data && corpora.data.items.length > 0 && (
          <Table>
            <THead>
              <tr>
                <Th>Корпус</Th>
                <Th className="text-right">Сессий</Th>
                <Th>Отбор</Th>
                <Th>Хеш</Th>
              </tr>
            </THead>
            <TBody>
              {corpora.data.items.map((c) => (
                <Tr key={c.id}>
                  <Td title={c.description}>
                    <span className="font-medium">{c.name}</span>
                    {c.description && <span className="ml-3 hidden max-w-[42ch] truncate align-bottom text-xs text-muted xl:inline-block">{c.description}</span>}
                  </Td>
                  <Td className="text-right font-mono text-xs">{c.sessionIds.length}</Td>
                  <Td className="text-xs text-muted">{describeCorpusFilter(c.filter).join(' · ')}</Td>
                  <Td className="font-mono text-xs text-muted">{shortHash(c.hash, 12)}</Td>
                </Tr>
              ))}
            </TBody>
          </Table>
        )}
      </Pane>

      <Pane>
        <PaneHeader title="Проверки" />
        {runs.isLoading && <Loader />}
        {runs.error && <ErrorState error={runs.error} onRetry={runs.refetch} />}
        {runs.data && runs.data.items.length === 0 && <EmptyState title="Проверок ещё не было" />}
        {runs.data && runs.data.items.length > 0 && (
          <Table>
            <THead>
              <tr>
                <Th>Проверка</Th>
                <Th>Интерпретация</Th>
                <Th>Корпус</Th>
                <Th title="Сообщений в корпусе и доли: разобрано, нарушения, неполные, неизвестный тип">Результат</Th>
                <Th />
                <Th>Запуск</Th>
              </tr>
            </THead>
            <TBody>
              {runs.data.items.map((r) => (
                <Tr key={r.id} onClick={() => navigate(routes.run(projectId, r.id))}>
                  <Td className="font-mono text-xs">{r.id}</Td>
                  <Td>
                    {r.interpretationName} <Tag>v{r.version}</Tag>
                  </Td>
                  <Td className="text-xs">{r.corpusName}</Td>
                  <Td>{r.summary ? <RunOutcome summary={r.summary} /> : <span className="text-muted">—</span>}</Td>
                  <Td>
                    <span className="flex flex-wrap gap-1">
                      <RunStatusMark status={r.status} />
                      {r.stale && <StaleTag reasons={r.staleReasons} />}
                    </span>
                  </Td>
                  <Td className="text-xs whitespace-nowrap text-muted">
                    {formatDateTime(r.startedAt)}
                    {r.durationMs !== undefined && <span className="ml-2 font-mono">{formatDuration(r.durationMs)}</span>}
                  </Td>
                </Tr>
              ))}
            </TBody>
          </Table>
        )}
      </Pane>
    </Page>
  );
}
