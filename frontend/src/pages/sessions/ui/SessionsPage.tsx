import { useQuery } from '@tanstack/react-query';
import { useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router';
import { actionQueries } from '@/entities/action';
import { captureQueries } from '@/entities/capture';
import { sessionQueries, type SessionFilters, type SessionStatus } from '@/entities/session';
import { SessionTable } from '@/widgets/session-table';
import { routes } from '@/shared/config';
import { useDebouncedValue, useRequiredParams } from '@/shared/lib';
import { Pane, Checkbox, EmptyState, ErrorState, Input, Loader, Page, PageHeader, Select } from '@/shared/ui';

export function SessionsPage() {
  const { projectId } = useRequiredParams('projectId');
  const [params, setParams] = useSearchParams();
  const navigate = useNavigate();
  const [q, setQ] = useState('');
  const query = useDebouncedValue(q);

  const filters: SessionFilters = {
    captureId: params.get('captureId') ?? undefined,
    status: (params.get('status') as SessionStatus | null) ?? undefined,
    target: params.get('all') === '1' ? undefined : true,
    q: query || undefined,
  };
  const sessions = useQuery(sessionQueries.list(projectId, filters));
  const captures = useQuery(captureQueries.list(projectId));
  const actions = useQuery(actionQueries.list(projectId));

  const setParam = (key: string, value: string | null) => {
    const next = new URLSearchParams(params);
    if (value) next.set(key, value);
    else next.delete(key);
    setParams(next, { replace: true });
  };

  return (
    <Page>
      <PageHeader
        title="TCP-сессии"
        description="Каждое соединение — отдельная сессия, даже при повторном использовании адресов и портов. Неполные сессии не скрываются: ошибки одной сессии не блокируют остальные."
      />
      <div className="mb-3 flex flex-wrap items-center gap-2">
        <Select value={filters.captureId ?? ''} onChange={(e) => setParam('captureId', e.target.value || null)} className="w-72">
          <option value="">Все записи</option>
          {captures.data?.items.map((c) => (
            <option key={c.id} value={c.id}>
              {c.fileName}
            </option>
          ))}
        </Select>
        <Select value={filters.status ?? ''} onChange={(e) => setParam('status', e.target.value || null)} className="w-44">
          <option value="">Любое состояние</option>
          <option value="complete">Полные</option>
          <option value="partial">Неполные</option>
          <option value="error">С ошибками</option>
        </Select>
        <Input value={q} onChange={(e) => setQ(e.target.value)} placeholder="IP, порт, id сессии…" className="w-60" />
        <Checkbox
          label="показать фоновый трафик"
          checked={params.get('all') === '1'}
          onChange={(e) => setParam('all', e.target.checked ? '1' : null)}
          className="ml-2"
        />
        <span className="ml-auto text-xs text-muted">{sessions.data ? `${sessions.data.total} сессий` : ''}</span>
      </div>
      {sessions.isLoading && <Loader />}
      {sessions.error && <ErrorState error={sessions.error} onRetry={sessions.refetch} />}
      {sessions.data && sessions.data.items.length === 0 && (
        <EmptyState title="Сессий не найдено" description="Измените фильтры или загрузите запись трафика." />
      )}
      {sessions.data && sessions.data.items.length > 0 && (
        <Pane>
          <SessionTable
            sessions={sessions.data.items}
            actions={actions.data?.items}
            onOpen={(s) => navigate(routes.session(projectId, s.id))}
          />
        </Pane>
      )}
    </Page>
  );
}
