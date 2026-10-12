import { ArrowLeftIcon } from '@phosphor-icons/react';
import { useQuery } from '@tanstack/react-query';
import { useState } from 'react';
import { Link } from 'react-router';
import { corpusQueries, describeCorpusFilter } from '@/entities/corpus';
import { hypothesisBehind, interpretationQueries } from '@/entities/interpretation';
import { FINDING_KIND, FINDING_KINDS, RunStatusMark, StaleTag, verificationQueries, type FindingKind } from '@/entities/verification';
import { AddCounterexampleButton } from '@/features/manage-hypothesis';
import { StartRunButton } from '@/features/run-verification';
import { FindingsTable, RuleStatsTable, RunReadout } from '@/widgets/verification-report';
import { routes } from '@/shared/config';
import { countLabel, formatDateTime, formatDuration, shortHash, useRequiredParams } from '@/shared/lib';
import { Disclosure, EmptyState, ErrorState, KeyValue, Loader, Mono, Note, Page, PageHeader, Pane, Progress, Tabs } from '@/shared/ui';

/** One table at a time: findings of a kind, or the same run cut by rule. */
type View = FindingKind | 'all' | 'rules';

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
  const version = useQuery({ ...interpretationQueries.version(run.data?.versionId ?? ''), enabled: !!run.data });
  const [view, setView] = useState<View>('all');

  if (run.isLoading) return <Loader />;
  if (run.error || !run.data) return <ErrorState error={run.error} onRetry={run.refetch} />;
  const r = run.data;
  const corpus = corpora.data?.items.find((c) => c.id === r.corpusId);
  const all = findings.data?.items ?? [];
  const visible = view === 'all' || view === 'rules' ? all : all.filter((f) => f.kind === view);
  const countOf = (k: FindingKind) => all.filter((f) => f.kind === k).length;

  return (
    <Page>
      <Link to={routes.verification(projectId)} className="mb-2 inline-flex items-center gap-1.5 text-xs text-muted hover:text-fg">
        <ArrowLeftIcon size={12} /> Все проверки
      </Link>
      <PageHeader
        className="mb-2"
        title={
          <span className="flex items-baseline gap-2">
            Проверка <Mono className="text-lg">{r.id}</Mono>
          </span>
        }
        meta={
          <>
            <RunStatusMark status={r.status} />
            {r.stale && <StaleTag reasons={r.staleReasons} />}
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

      {/* What was checked, on one line; hashes and engine settings are one click away. */}
      <p className="flex flex-wrap items-baseline gap-x-2 gap-y-0.5 text-xs text-muted">
        <Link to={routes.spec(projectId, { interpretationId: r.interpretationId, versionId: r.versionId })} className="link text-fg">
          {r.interpretationName} v{r.version}
        </Link>
        <span aria-hidden>·</span>
        <span>
          {r.corpusName}
          {corpus ? `, ${countLabel(corpus.sessionIds.length, ['сессия', 'сессии', 'сессий'])}` : ''}
        </span>
        <span aria-hidden>·</span>
        <span>{formatDateTime(r.startedAt)}</span>
        {r.durationMs !== undefined && (
          <>
            <span aria-hidden>·</span>
            <span className="font-mono">{formatDuration(r.durationMs)}</span>
          </>
        )}
        {previous.data && (
          <>
            <span aria-hidden>·</span>
            <span title="Числа в сводке показаны в сравнении с этой проверкой той же интерпретации на этом корпусе">
              сравнение с{' '}
              <Link to={routes.run(projectId, previous.data.id)} className="link">
                {previous.data.id} (v{previous.data.version})
              </Link>
            </span>
          </>
        )}
      </p>
      <Disclosure summary="Условия воспроизведения" className="mt-1.5">
        <div className="grid gap-x-10 gap-y-1 border-l border-line pl-4 lg:grid-cols-2">
          <KeyValue
            items={[
              ['Хеш версии', <Mono key="h">{shortHash(r.versionHash, 24)}</Mono>],
              ['Корпус', <Mono key="c">{shortHash(r.corpusHash, 24)}</Mono>],
              ['Отбор', corpus ? describeCorpusFilter(corpus.filter).join(' · ') : '—'],
            ]}
          />
          <KeyValue
            items={[
              ['Движок', <Mono key="e">{r.engineVersion}</Mono>],
              ['Перекрытия', r.settings.overlapPolicy],
              ['Checksum', r.settings.checksumPolicy],
            ]}
          />
        </div>
      </Disclosure>

      {r.stale && (
        <Note tone="warn" title="Результат устарел и не отражает текущее состояние" className="mt-4">
          <ul className="list-inside list-disc">
            {r.staleReasons.map((reason) => (
              <li key={reason}>{reason}</li>
            ))}
          </ul>
        </Note>
      )}

      {(r.status === 'running' || r.status === 'queued') && (
        <Pane className="mt-4 px-4 py-4">
          <p className="mb-2 text-sm">Проверка выполняется…</p>
          <Progress value={0.5} className="animate-pulse" />
        </Pane>
      )}

      {r.summary && (
        <div className="mt-4">
          <RunReadout summary={r.summary} previous={previous.data?.summary} />
          <Note tone="info" className="mt-3">
            Совпадение на выбранных примерах не доказывает назначение полей и универсальность модели. Неприменимые правила
            {r.summary.inapplicableRules.length ? ` (${r.summary.inapplicableRules.join(', ')})` : ''} на этом корпусе не проверены.
          </Note>

          <Pane className="mt-4 min-w-0">
            <Tabs<View>
              value={view}
              onChange={setView}
              items={[
                { id: 'all', label: 'Все находки', count: all.length },
                ...FINDING_KINDS.filter((k) => countOf(k) > 0).map((k) => ({ id: k, label: FINDING_KIND[k].label, count: countOf(k) })),
                { id: 'rules', label: 'По правилам', count: r.summary.byRule.length },
              ]}
            />
            {view === 'rules' ? (
              <RuleStatsTable summary={r.summary} />
            ) : findings.isLoading ? (
              <Loader />
            ) : (
              <FindingsTable
                projectId={projectId}
                findings={visible}
                versionId={r.versionId}
                empty={view === 'all' ? 'Находок нет' : undefined}
                exampleAction={(f, ex) =>
                  f.kind === 'violation' && (
                    <AddCounterexampleButton
                      projectId={projectId}
                      source={ex}
                      runId={r.id}
                      defaultNote={f.message}
                      defaultHypothesisId={version.data && hypothesisBehind(version.data.spec, f.ruleId, f.fieldPath)}
                    />
                  )
                }
              />
            )}
          </Pane>
        </div>
      )}
      {r.status === 'cancelled' && <EmptyState title="Проверка отменена" description="Результаты не сформированы." />}
      {r.status === 'failed' && <EmptyState title="Проверка не выполнена" description={r.error ?? 'Результаты не сформированы.'} />}
    </Page>
  );
}
