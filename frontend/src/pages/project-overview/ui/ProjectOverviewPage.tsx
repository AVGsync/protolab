import { useQuery } from '@tanstack/react-query';
import { CheckCircle2, Circle } from 'lucide-react';
import { Link } from 'react-router';
import { HYPOTHESIS_STATUSES, HypothesisStatusBadge, hypothesisQueries } from '@/entities/hypothesis';
import { observationQueries, SourceRefLink } from '@/entities/observation';
import { projectQueries } from '@/entities/project';
import { reportQueries } from '@/entities/report';
import { RunStatusBadge, StaleBadge, verificationQueries } from '@/entities/verification';
import { ExportProjectButton } from '@/features/export-project';
import { ProjectSettingsButton } from '@/features/project-settings';
import { routes } from '@/shared/config';
import { cn, formatDateTime, useRequiredParams } from '@/shared/lib';
import { Badge, Card, CardBody, CardHeader, ErrorState, KeyValue, Loader, Page, PageHeader, Stat } from '@/shared/ui';

export function ProjectOverviewPage() {
  const { projectId } = useRequiredParams('projectId');
  const project = useQuery(projectQueries.detail(projectId));
  const hypotheses = useQuery(hypothesisQueries.list(projectId));
  const runs = useQuery(verificationQueries.runs(projectId));
  const observations = useQuery(observationQueries.list(projectId));
  const reports = useQuery(reportQueries.list(projectId));

  if (project.isLoading) return <Loader />;
  if (project.error || !project.data) return <ErrorState error={project.error} onRetry={project.refetch} />;
  const p = project.data;

  const steps = [
    { done: p.stats.captures > 0, title: 'Загрузить записи трафика', to: routes.captures(projectId) },
    { done: p.stats.actions > 0, title: 'Импортировать журнал действий', to: routes.actions(projectId) },
    { done: p.stats.observations > 0, title: 'Изучить потоки и зафиксировать наблюдения', to: routes.sessions(projectId) },
    { done: p.stats.interpretations > 0, title: 'Сформулировать интерпретацию', to: routes.interpretations(projectId) },
    { done: p.stats.runs > 0, title: 'Проверить на корпусе и дополнительной записи', to: routes.verification(projectId) },
    { done: (reports.data?.total ?? 0) > 0, title: 'Сформировать отчёт и передать проект', to: routes.reports(projectId) },
  ];

  const byStatus = HYPOTHESIS_STATUSES.map((s) => ({
    status: s,
    count: hypotheses.data?.items.filter((h) => h.status === s).length ?? 0,
  }));

  return (
    <Page>
      <PageHeader
        title={p.name}
        description={p.description}
        actions={
          <>
            <ProjectSettingsButton project={p} />
            <ExportProjectButton projectId={projectId} />
          </>
        }
      />

      <div className="grid grid-cols-2 gap-3 md:grid-cols-3 xl:grid-cols-6">
        <Stat label="Записи" value={p.stats.captures} />
        <Stat label="TCP-сессии" value={p.stats.sessions} />
        <Stat label="Действия" value={p.stats.actions} />
        <Stat label="Наблюдения" value={p.stats.observations} />
        <Stat label="Гипотезы" value={p.stats.hypotheses} />
        <Stat label="Проверки" value={p.stats.runs} />
      </div>

      <div className="mt-5 grid gap-4 lg:grid-cols-3">
        <Card>
          <CardHeader title="Ход исследования" description="Порядок может меняться по мере находок" />
          <CardBody className="flex flex-col gap-1">
            {steps.map((s, i) => (
              <Link key={s.title} to={s.to} className="flex items-center gap-2.5 rounded-md px-2 py-1.5 hover:bg-surface-2">
                {s.done ? <CheckCircle2 size={16} className="text-success" /> : <Circle size={16} className="text-subtle" />}
                <span className={cn('text-[13px]', !s.done && 'text-muted')}>
                  {i + 1}. {s.title}
                </span>
              </Link>
            ))}
          </CardBody>
        </Card>

        <Card>
          <CardHeader
            title="Гипотезы"
            actions={
              <Link to={routes.hypotheses(projectId)} className="text-xs text-info hover:underline">
                все
              </Link>
            }
          />
          <CardBody className="flex flex-col gap-3">
            <div className="flex flex-wrap gap-2">
              {byStatus.map((b) => (
                <span key={b.status} className="inline-flex items-center gap-1.5">
                  <HypothesisStatusBadge status={b.status} />
                  <span className="font-mono text-xs">{b.count}</span>
                </span>
              ))}
            </div>
            <ul className="flex flex-col gap-1.5">
              {hypotheses.data?.items
                .filter((h) => h.status === 'testing' || h.status === 'proposed')
                .slice(0, 4)
                .map((h) => (
                  <li key={h.id}>
                    <Link to={routes.hypotheses(projectId, h.id)} className="text-[12.5px] hover:text-info">
                      {h.title}
                    </Link>
                  </li>
                ))}
            </ul>
          </CardBody>
        </Card>

        <Card>
          <CardHeader title="Восстановление потоков" actions={<ProjectSettingsButton project={p} />} />
          <CardBody>
            <KeyValue
              items={[
                ['Перекрытия', p.settings.overlapPolicy === 'first' ? 'первый пакет' : 'последний пакет'],
                ['Checksum TCP', p.settings.checksumPolicy === 'warn' ? 'предупреждать' : 'игнорировать'],
                ['Порты устройства', <span key="p" className="font-mono">{p.settings.targetPorts.join(', ') || '—'}</span>],
                ['Создан', formatDateTime(p.createdAt)],
              ]}
            />
          </CardBody>
        </Card>
      </div>

      <div className="mt-4 grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader
            title="Последние проверки"
            actions={
              <Link to={routes.verification(projectId)} className="text-xs text-info hover:underline">
                все
              </Link>
            }
          />
          <ul className="divide-y divide-line">
            {runs.data?.items.slice(0, 4).map((r) => (
              <li key={r.id}>
                <Link to={routes.run(projectId, r.id)} className="flex items-center gap-3 px-4 py-2.5 hover:bg-surface-2">
                  <span className="font-mono text-xs text-subtle">{r.id}</span>
                  <span className="flex-1 text-[12.5px]">
                    {r.interpretationName} v{r.version} → «{r.corpusName}»
                  </span>
                  {r.summary && r.summary.violations > 0 && <Badge tone="danger">нарушений: {r.summary.violations}</Badge>}
                  {r.stale && <StaleBadge reasons={r.staleReasons} />}
                  <RunStatusBadge status={r.status} />
                </Link>
              </li>
            ))}
            {runs.data?.items.length === 0 && <li className="px-4 py-6 text-center text-xs text-muted">Проверок ещё не было</li>}
          </ul>
        </Card>

        <Card>
          <CardHeader
            title="Последние наблюдения"
            actions={
              <Link to={routes.observations(projectId)} className="text-xs text-info hover:underline">
                все
              </Link>
            }
          />
          <ul className="divide-y divide-line">
            {observations.data?.items.slice(0, 5).map((o) => (
              <li key={o.id} className="px-4 py-2.5">
                <p className="text-[12.5px]">{o.note}</p>
                <SourceRefLink projectId={projectId} source={o.source} />
              </li>
            ))}
            {observations.data?.items.length === 0 && <li className="px-4 py-6 text-center text-xs text-muted">Наблюдений ещё нет</li>}
          </ul>
        </Card>
      </div>
    </Page>
  );
}
