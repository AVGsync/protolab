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
import { EmptyState, ErrorState, Loader, Note, Page, PageHeader, Pane, SectionTitle, Status, Tag } from '@/shared/ui';

export function HypothesesPage() {
  const { projectId } = useRequiredParams('projectId');
  const [params, setParams] = useSearchParams();
  const [filter, setFilter] = useState<HypothesisStatus | 'all'>('all');
  const { data, isLoading, error, refetch } = useQuery(hypothesisQueries.list(projectId));

  const items = (data?.items ?? []).filter((h) => filter === 'all' || h.status === filter);
  const selectedId = params.get('h') ?? items[0]?.id;
  const selected = data?.items.find((h) => h.id === selectedId);

  return (
    <Page className="max-w-none">
      <PageHeader
        title="Гипотезы"
        actions={<CreateHypothesisButton projectId={projectId} onCreated={(id) => setParams({ h: id })} />}
      />
      <div className="mb-3 flex flex-wrap gap-1.5">
        {(['all', ...HYPOTHESIS_STATUSES] as const).map((s) => (
          <button
            key={s}
            type="button"
            onClick={() => setFilter(s)}
            className={cn(
              'border px-2.5 py-1 text-xs',
              filter === s ? 'border-fg bg-surface-2 text-fg' : 'border-line text-muted hover:text-fg',
            )}
          >
            {s === 'all' ? 'Все' : HYPOTHESIS_STATUS[s].label}
            <span className="ml-1.5 font-mono text-subtle">
              {s === 'all' ? data?.total ?? 0 : data?.items.filter((h) => h.status === s).length ?? 0}
            </span>
          </button>
        ))}
      </div>
      {isLoading && <Loader />}
      {error && <ErrorState error={error} onRetry={refetch} />}
      {data && data.items.length === 0 && <EmptyState title="Гипотез пока нет" />}
      {data && data.items.length > 0 && (
        <div className="grid gap-4 lg:grid-cols-[380px_1fr]">
          <Pane className="self-start">
            <ul className="divide-y divide-line">
              {items.map((h) => (
                <li key={h.id}>
                  <button
                    type="button"
                    onClick={() => setParams({ h: h.id })}
                    className={cn('w-full px-4 py-3 text-left hover:bg-surface-2', h.id === selectedId && 'bg-selection')}
                  >
                    <div className="flex items-center justify-between gap-2">
                      <span className="font-mono text-[11px] text-subtle">{h.id}</span>
                      <HypothesisStatusMark status={h.status} />
                    </div>
                    <p className="mt-1 text-[13px]">{h.title}</p>
                    <p className="mt-1 text-[11px] text-subtle">
                      наблюдений: {h.observationIds.length} · контрпримеров: {h.counterexamples.length}
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
    <Pane className="flex flex-col gap-5 px-5 py-4">
      <div>
        <div className="flex items-center gap-2">
          <span className="font-mono text-xs text-subtle">{h.id}</span>
          <HypothesisStatusMark status={h.status} />
          <span className="ml-auto text-[11px] text-subtle">обновлена {formatDateTime(h.updatedAt)}</span>
        </div>
        <h2 className="mt-1.5 text-base font-semibold">{h.title}</h2>
        <p className="mt-1 text-[13px] text-fg/85">{h.statement}</p>
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

      <HypothesisStatusForm key={h.id + h.status} hypothesis={h} />

      <div>
        <SectionTitle>Основания — наблюдения ({observations.data?.total ?? h.observationIds.length})</SectionTitle>
        <ul className="flex flex-col gap-2">
          {observations.data?.items.map((o) => (
            <li key={o.id} className="border border-line bg-bg px-3 py-2">
              <p className="text-[12.5px]">{o.note}</p>
              <div className="mt-1 flex flex-wrap items-center gap-2">
                <SourceRefLink projectId={projectId} source={o.source} />
                <code className="font-mono text-[11px] text-fg">{o.snapshotHex}</code>
                {o.integrity === 'mismatch' && <Status tone="danger">байты источника изменились</Status>}
              </div>
            </li>
          ))}
          {observations.data?.items.length === 0 && <p className="text-xs text-subtle">Нет связанных наблюдений.</p>}
        </ul>
      </div>

      <div>
        <SectionTitle>Контрпримеры ({h.counterexamples.length})</SectionTitle>
        {h.counterexamples.length === 0 ? (
          <p className="text-xs text-subtle">Контрпримеров не найдено. Это не доказательство — расширьте корпус.</p>
        ) : (
          <ul className="flex flex-col gap-2">
            {h.counterexamples.map((c, i) => (
              <li key={i} className="border border-danger/40 px-3 py-2">
                <p className="text-[12.5px]">{c.note}</p>
                <div className="mt-1 flex items-center gap-3">
                  <SourceRefLink projectId={projectId} source={c.source} />
                  {c.runId && (
                    <Link to={routes.run(projectId, c.runId)} className="text-[11.5px] link">
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
          <ul className="list-inside list-disc text-[12.5px] text-muted">
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
                <span className="text-[11px] text-subtle">{formatDateTime(e.at)}</span>
              </div>
              <p className="mt-0.5 text-[12.5px] text-fg/85">{e.note}</p>
            </li>
          ))}
        </ol>
      </div>
    </Pane>
  );
}
