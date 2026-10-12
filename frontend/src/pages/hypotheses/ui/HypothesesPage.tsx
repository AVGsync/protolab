import { useQuery } from '@tanstack/react-query';
import { useState } from 'react';
import { Link, useSearchParams } from 'react-router';
import {
  HYPOTHESIS_STATUS,
  HYPOTHESIS_STATUSES,
  HypothesisStatusMark,
  hypothesisQueries,
  type Hypothesis,
  type HypothesisStatus,
} from '@/entities/hypothesis';
import { observationQueries, SourceRefLink } from '@/entities/observation';
import { CreateHypothesisButton, HypothesisStatusForm } from '@/features/manage-hypothesis';
import { routes } from '@/shared/config';
import { cn, formatDateTime, useRequiredParams } from '@/shared/lib';
import { Disclosure, EmptyState, ErrorState, Loader, Note, Page, PageHeader, Pane, SectionTitle, Segmented, Status, Tag } from '@/shared/ui';

export function HypothesesPage() {
  const { projectId } = useRequiredParams('projectId');
  const [params, setParams] = useSearchParams();
  const [filter, setFilter] = useState<HypothesisStatus | 'all'>('all');
  const { data, isLoading, error, refetch } = useQuery(hypothesisQueries.list(projectId));

  const items = (data?.items ?? []).filter((h) => filter === 'all' || h.status === filter);
  const selectedId = params.get('h') ?? items[0]?.id;
  const selected = data?.items.find((h) => h.id === selectedId);

  return (
    <Page>
      <PageHeader
        title="Гипотезы"
        actions={<CreateHypothesisButton projectId={projectId} onCreated={(id) => setParams({ h: id })} />}
      />
      <Segmented<HypothesisStatus | 'all'>
        className="mb-4"
        aria-label="Статус гипотезы"
        value={filter}
        onChange={setFilter}
        items={(['all', ...HYPOTHESIS_STATUSES] as const).map((s) => ({
          id: s,
          label: (
            <>
              {s === 'all' ? 'Все' : HYPOTHESIS_STATUS[s].label}
              <span className="font-mono text-2xs text-muted">
                {s === 'all' ? (data?.total ?? 0) : (data?.items.filter((h) => h.status === s).length ?? 0)}
              </span>
            </>
          ),
        }))}
      />
      {isLoading && <Loader />}
      {error && <ErrorState error={error} onRetry={refetch} />}
      {data && data.items.length === 0 && <EmptyState title="Гипотез пока нет" />}
      {data && data.items.length > 0 && (
        <div className="grid gap-4 lg:grid-cols-[360px_minmax(0,1fr)]">
          <Pane className="self-start">
            <ul className="divide-y divide-line">
              {items.map((h) => (
                <li key={h.id}>
                  <button
                    type="button"
                    onClick={() => setParams({ h: h.id })}
                    aria-current={h.id === selectedId}
                    className={cn('w-full px-3 py-2 text-left hover:bg-surface-2', h.id === selectedId && 'bg-selection hover:bg-selection')}
                  >
                    <p className="text-sm">{h.title}</p>
                    <p className="mt-0.5 flex flex-wrap items-center gap-x-3 text-2xs text-muted">
                      <span className="font-mono">{h.id}</span>
                      <HypothesisStatusMark status={h.status} />
                      <span title="Наблюдения / контрпримеры">
                        <span className="font-mono">{h.observationIds.length}</span> набл. ·{' '}
                        <span className={cn('font-mono', h.counterexamples.length > 0 && 'text-danger')}>{h.counterexamples.length}</span> контрпр.
                      </span>
                    </p>
                  </button>
                </li>
              ))}
            </ul>
          </Pane>
          {selected ? <HypothesisDetails projectId={projectId} hypothesis={selected} /> : <EmptyState title="Выберите гипотезу" />}
        </div>
      )}
    </Page>
  );
}

function HypothesisDetails({ projectId, hypothesis: h }: { projectId: string; hypothesis: Hypothesis }) {
  const observations = useQuery(observationQueries.list(projectId, { hypothesisId: h.id }));
  return (
    <Pane className="flex flex-col gap-5 px-4 py-4">
      <div>
        <div className="flex items-center gap-2">
          <span className="font-mono text-xs text-muted">{h.id}</span>
          <HypothesisStatusMark status={h.status} />
          <span className="ml-auto text-2xs text-muted">обновлена {formatDateTime(h.updatedAt)}</span>
        </div>
        <h2 className="mt-1.5 text-lg font-semibold">{h.title}</h2>
        <p className="mt-1 text-sm text-fg/85">{h.statement}</p>
        {h.ruleRefs.length > 0 && (
          <div className="mt-2 flex flex-wrap gap-1">
            {h.ruleRefs.map((r) => (
              <Tag key={r}>{r}</Tag>
            ))}
          </div>
        )}
      </div>

      {h.status === 'supported' && (
        <Note tone="info">Совпадение на выбранных примерах не является доказательством назначения поля — проверяйте на новых записях.</Note>
      )}

      <Disclosure summary="Сменить статус">
        <HypothesisStatusForm key={h.id + h.status} hypothesis={h} />
      </Disclosure>

      <div>
        <SectionTitle>Основания — наблюдения ({observations.data?.total ?? h.observationIds.length})</SectionTitle>
        <ul className="flex flex-col divide-y divide-line border-y border-line">
          {observations.data?.items.map((o) => (
            <li key={o.id} className="py-2">
              <p className="text-xs">{o.note}</p>
              <div className="mt-1 flex flex-wrap items-center gap-2">
                <SourceRefLink projectId={projectId} source={o.source} />
                <code className="font-mono text-2xs text-fg">{o.snapshotHex}</code>
                {o.integrity === 'mismatch' && <Status tone="danger">байты источника изменились</Status>}
              </div>
            </li>
          ))}
          {observations.data?.items.length === 0 && <p className="text-xs text-muted">Нет связанных наблюдений.</p>}
        </ul>
      </div>

      <div>
        <SectionTitle>Контрпримеры ({h.counterexamples.length})</SectionTitle>
        {h.counterexamples.length === 0 ? (
          <p className="text-xs text-muted">Контрпримеров не найдено. Это не доказательство — расширьте корпус.</p>
        ) : (
          <ul className="flex flex-col gap-3">
            {h.counterexamples.map((c, i) => (
              <li key={i} className="border-l-2 border-danger py-0.5 pl-3">
                <p className="text-xs">{c.note}</p>
                <div className="mt-1 flex items-center gap-3">
                  <SourceRefLink projectId={projectId} source={c.source} />
                  {c.runId && (
                    <Link to={routes.run(projectId, c.runId)} className="text-2xs link">
                      проверка {c.runId}
                    </Link>
                  )}
                </div>
              </li>
            ))}
          </ul>
        )}
      </div>

      {h.alternatives.length > 0 && (
        <div>
          <SectionTitle>Альтернативные объяснения</SectionTitle>
          <ul className="list-inside list-disc text-xs text-muted">
            {h.alternatives.map((a) => (
              <li key={a}>{a}</li>
            ))}
          </ul>
        </div>
      )}

      <div>
        <SectionTitle>История</SectionTitle>
        <ol className="relative ml-1.5 border-l border-line">
          {h.history.map((e, i) => (
            <li key={i} className="mb-3 ml-4">
              <span className="absolute -left-[5px] mt-1.5 size-2.5 border border-line-strong bg-surface-2" />
              <div className="flex items-center gap-2">
                <HypothesisStatusMark status={e.status} />
                <span className="text-2xs text-muted">{formatDateTime(e.at)}</span>
              </div>
              <p className="mt-0.5 text-xs text-fg/85">{e.note}</p>
            </li>
          ))}
        </ol>
      </div>
    </Pane>
  );
}
