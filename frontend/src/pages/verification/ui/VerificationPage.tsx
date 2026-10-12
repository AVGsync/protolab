import { useQuery } from '@tanstack/react-query';
import { FlaskConical } from 'lucide-react';
import { useNavigate } from 'react-router';
import { corpusQueries, describeCorpusFilter } from '@/entities/corpus';
import { RunStatusBadge, StaleBadge, verificationQueries } from '@/entities/verification';
import { CreateCorpusButton } from '@/features/create-corpus';
import { StartRunButton } from '@/features/run-verification';
import { routes } from '@/shared/config';
import { formatDateTime, formatDuration, shortHash, useRequiredParams } from '@/shared/lib';
import { Badge, Card, CardHeader, EmptyState, ErrorState, Loader, Page, PageHeader, Table, TBody, Td, Th, THead, Tr } from '@/shared/ui';

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
        description="Интерпретация применяется к корпусу целиком. Сводка показывает нарушения, неполные и неоднозначные данные, неизвестные участки и неприменимые правила — и условия отбора корпуса."
        actions={
          <>
            <CreateCorpusButton projectId={projectId} />
            <StartRunButton projectId={projectId} />
          </>
        }
      />

      <Card className="mb-5">
        <CardHeader title="Корпуса" description="Зафиксированные наборы сессий; хеш меняется при изменении состава" />
        {corpora.isLoading && <Loader />}
        <div className="grid gap-3 p-4 md:grid-cols-3">
          {corpora.data?.items.map((c) => (
            <div key={c.id} className="rounded-md border border-line bg-bg px-3 py-2.5">
              <div className="flex items-center justify-between gap-2">
                <p className="font-medium">{c.name}</p>
                <Badge>{c.sessionIds.length} сес.</Badge>
              </div>
              <p className="mt-0.5 text-[12px] text-muted">{c.description}</p>
              <p className="mt-1.5 text-[11px] text-subtle">{describeCorpusFilter(c.filter).join(' · ')}</p>
              <p className="font-mono text-[10.5px] text-subtle">hash {shortHash(c.hash, 12)}</p>
            </div>
          ))}
          {corpora.data?.items.length === 0 && <p className="text-xs text-muted">Создайте корпус для проверки.</p>}
        </div>
      </Card>

      <Card>
        <CardHeader title="Проверки" />
        {runs.isLoading && <Loader />}
        {runs.error && <ErrorState error={runs.error} onRetry={runs.refetch} />}
        {runs.data && runs.data.items.length === 0 && <EmptyState icon={<FlaskConical size={20} />} title="Проверок ещё не было" />}
        {runs.data && runs.data.items.length > 0 && (
          <Table>
            <THead>
              <tr>
                <Th>Проверка</Th>
                <Th>Интерпретация</Th>
                <Th>Корпус</Th>
                <Th className="text-right">Сообщ.</Th>
                <Th className="text-right">OK</Th>
                <Th className="text-right">Наруш.</Th>
                <Th className="text-right">Неполн.</Th>
                <Th className="text-right">Неизв.</Th>
                <Th>Состояние</Th>
                <Th>Запуск</Th>
              </tr>
            </THead>
            <TBody>
              {runs.data.items.map((r) => (
                <Tr key={r.id} onClick={() => navigate(routes.run(projectId, r.id))}>
                  <Td className="font-mono text-[12px]">{r.id}</Td>
                  <Td>
                    {r.interpretationName} <span className="rounded bg-surface-3 px-1 font-mono text-[11px]">v{r.version}</span>
                  </Td>
                  <Td className="text-[12.5px]">{r.corpusName}</Td>
                  <Td className="text-right font-mono">{r.summary?.messages ?? '—'}</Td>
                  <Td className="text-right font-mono text-success">{r.summary?.ok ?? ''}</Td>
                  <Td className="text-right font-mono text-danger">{r.summary?.violations || ''}</Td>
                  <Td className="text-right font-mono text-warning">{r.summary?.partial || ''}</Td>
                  <Td className="text-right font-mono text-muted">{r.summary?.unknownType || ''}</Td>
                  <Td>
                    <span className="flex flex-wrap gap-1">
                      <RunStatusBadge status={r.status} />
                      {r.stale && <StaleBadge reasons={r.staleReasons} />}
                    </span>
                  </Td>
                  <Td className="text-[12px] whitespace-nowrap text-muted">
                    {formatDateTime(r.startedAt)}
                    {r.durationMs !== undefined && <div className="text-[11px] text-subtle">{formatDuration(r.durationMs)}</div>}
                  </Td>
                </Tr>
              ))}
            </TBody>
          </Table>
        )}
      </Card>
    </Page>
  );
}
