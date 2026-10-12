import { useQuery } from '@tanstack/react-query';
import { ArrowLeft, Clock3 } from 'lucide-react';
import { useState } from 'react';
import { Link } from 'react-router';
import { corpusQueries, describeCorpusFilter } from '@/entities/corpus';
import { interpretationQueries } from '@/entities/interpretation';
import { FINDING_KIND, FINDING_KINDS, RunStatusBadge, StaleBadge, verificationQueries, type FindingKind } from '@/entities/verification';
import { StartRunButton } from '@/features/run-verification';
import { FindingsTable, RuleStatsTable, RunSummaryCards } from '@/widgets/verification-report';
import { routes } from '@/shared/config';
import { formatDateTime, formatDuration, shortHash, useRequiredParams } from '@/shared/lib';
import { Callout, Card, CardBody, CardHeader, EmptyState, ErrorState, KeyValue, Loader, Mono, Page, PageHeader, Progress, Tabs } from '@/shared/ui';

export function RunDetailsPage() {
  const { projectId, runId } = useRequiredParams('projectId', 'runId');
  const run = useQuery({
    ...verificationQueries.run(runId),
    refetchInterval: (q) => (q.state.data?.status === 'running' || q.state.data?.status === 'queued' ? 1000 : false),
  });
  const findings = useQuery({ ...verificationQueries.findings(runId), enabled: run.data?.status === 'succeeded' });
  const previous = useQuery({ ...verificationQueries.run(run.data?.previousRunId ?? ''), enabled: !!run.data?.previousRunId });
  const corpora = useQuery(corpusQueries.list(projectId));
  const interp = useQuery({ ...interpretationQueries.detail(run.data?.interpretationId ?? ''), enabled: !!run.data });
  const [kind, setKind] = useState<FindingKind | 'all'>('all');

  if (run.isLoading) return <Loader />;
  if (run.error || !run.data) return <ErrorState error={run.error} onRetry={run.refetch} />;
  const r = run.data;
  const corpus = corpora.data?.items.find((c) => c.id === r.corpusId);
  const all = findings.data?.items ?? [];
  const visible = kind === 'all' ? all : all.filter((f) => f.kind === kind);
  const countOf = (k: FindingKind) => all.filter((f) => f.kind === k).length;

  return (
    <Page className="max-w-none">
      <Link to={routes.verification(projectId)} className="mb-3 inline-flex items-center gap-1.5 text-xs text-muted hover:text-fg">
        <ArrowLeft size={13} /> Все проверки
      </Link>
      <PageHeader
        title={
          <span className="flex items-center gap-2">
            Проверка <Mono className="text-base">{r.id}</Mono>
          </span>
        }
        meta={
          <>
            <RunStatusBadge status={r.status} />
            {r.stale && <StaleBadge reasons={r.staleReasons} />}
          </>
        }
        actions={
          interp.data && (
            <StartRunButton
              projectId={projectId}
              defaultVersionId={interp.data.latestVersionId}
              defaultCorpusId={r.corpusId}
              label={r.stale ? `Перепроверить на v${interp.data.latestVersion}` : 'Повторить'}
              variant={r.stale ? 'primary' : 'secondary'}
            />
          )
        }
      />

      {r.stale && (
        <Callout tone="warning" icon={<Clock3 size={14} />} title="Результат устарел и не отражает текущее состояние" className="mb-4">
          <ul className="list-inside list-disc">
            {r.staleReasons.map((reason) => (
              <li key={reason}>{reason}</li>
            ))}
          </ul>
        </Callout>
      )}

      <div className="mb-4 grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader title="Что проверялось" />
          <CardBody>
            <KeyValue
              items={[
                [
                  'Интерпретация',
                  <Link key="i" to={routes.interpretation(projectId, r.interpretationId, r.versionId)} className="text-info hover:underline">
                    {r.interpretationName} v{r.version}
                  </Link>,
                ],
                ['Хеш версии', <Mono key="h">{shortHash(r.versionHash, 24)}</Mono>],
                ['Корпус', `${r.corpusName}${corpus ? ` — ${corpus.sessionIds.length} сессий` : ''}`],
                ['Условия отбора', corpus ? describeCorpusFilter(corpus.filter).join(' · ') : '—'],
                ['Хеш корпуса', <Mono key="c">{shortHash(r.corpusHash, 24)}</Mono>],
              ]}
            />
          </CardBody>
        </Card>
        <Card>
          <CardHeader title="Условия воспроизведения" />
          <CardBody>
            <KeyValue
              items={[
                ['Движок', <Mono key="e">{r.engineVersion}</Mono>],
                ['Перекрытия', r.settings.overlapPolicy],
                ['Checksum', r.settings.checksumPolicy],
                ['Запуск', formatDateTime(r.startedAt)],
                ['Длительность', r.durationMs !== undefined ? formatDuration(r.durationMs) : '—'],
              ]}
            />
          </CardBody>
        </Card>
      </div>

      {(r.status === 'running' || r.status === 'queued') && (
        <Card className="mb-4 px-4 py-4">
          <p className="mb-2 text-[13px]">Проверка выполняется…</p>
          <Progress value={0.5} tone="info" className="animate-pulse" />
        </Card>
      )}

      {r.summary && (
        <>
          <RunSummaryCards summary={r.summary} previous={previous.data?.summary} />
          {previous.data && (
            <p className="mt-2 text-[11.5px] text-subtle">
              Сравнение с предыдущей проверкой той же интерпретации на этом корпусе:{' '}
              <Link to={routes.run(projectId, previous.data.id)} className="text-info hover:underline">
                {previous.data.id} (v{previous.data.version})
              </Link>
            </p>
          )}
          <Callout tone="info" className="mt-4">
            Совпадение на выбранных примерах не доказывает назначение полей и универсальность модели. Неприменимые правила
            {r.summary.inapplicableRules.length ? ` (${r.summary.inapplicableRules.join(', ')})` : ''} на этом корпусе не проверены.
          </Callout>

          <div className="mt-4 grid gap-4 xl:grid-cols-[1fr_340px]">
            <Card className="min-w-0">
              <Tabs<FindingKind | 'all'>
                value={kind}
                onChange={setKind}
                items={[
                  { id: 'all', label: 'Все', badge: all.length },
                  ...FINDING_KINDS.filter((k) => countOf(k) > 0).map((k) => ({ id: k, label: FINDING_KIND[k].label, badge: countOf(k) })),
                ]}
              />
              {findings.isLoading ? <Loader /> : <FindingsTable projectId={projectId} findings={visible} versionId={r.versionId} />}
            </Card>
            <Card className="self-start">
              <CardHeader title="По правилам" />
              <RuleStatsTable summary={r.summary} />
            </Card>
          </div>
        </>
      )}
      {r.status === 'cancelled' && <EmptyState title="Проверка отменена" description="Результаты не сформированы." />}
    </Page>
  );
}
