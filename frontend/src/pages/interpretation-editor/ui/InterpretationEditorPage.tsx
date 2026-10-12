import { useQuery } from '@tanstack/react-query';
import { useState } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router';
import { hypothesisQueries } from '@/entities/hypothesis';
import {
  interpretationApi,
  interpretationQueries,
  type Interpretation,
  type InterpretationVersion,
} from '@/entities/interpretation';
import { sessionQueries } from '@/entities/session';
import { CreateInterpretationButton, SaveVersionButton, SpecEditor, useSpecDraft } from '@/features/edit-interpretation';
import { StartRunButton } from '@/features/run-verification';
import { FieldTable, MessageList } from '@/widgets/message-panel';
import { SpecOutline } from '@/widgets/spec-outline';
import type { ListResponse } from '@/shared/api';
import { routes } from '@/shared/config';
import { cn, formatDateTime, shortHash, useDebouncedValue, useRequiredParams } from '@/shared/lib';
import { Arrow, DownloadButton, EmptyState, ErrorState, Loader, Note, Select, Status, Tabs, Tag } from '@/shared/ui';

export function InterpretationEditorPage() {
  const { projectId } = useRequiredParams('projectId');
  const [params] = useSearchParams();
  const list = useQuery(interpretationQueries.list(projectId));
  // The editor shows one interpretation: the one in ?i=, else the first.
  const interpretationId = params.get('i') ?? list.data?.items[0]?.id ?? '';
  const hasInterpretation = !!interpretationId;
  const interp = useQuery({ ...interpretationQueries.detail(interpretationId), enabled: hasInterpretation });
  const versions = useQuery({ ...interpretationQueries.versions(interpretationId), enabled: hasInterpretation });
  const versionId = params.get('v') ?? interp.data?.latestVersionId;
  const version = useQuery({ ...interpretationQueries.version(versionId ?? ''), enabled: !!versionId });

  if (list.isLoading) return <Loader />;
  if (list.error) return <ErrorState error={list.error} onRetry={list.refetch} />;
  if (!hasInterpretation)
    return (
      <EmptyState
        title="Интерпретаций нет"
        description="Даже частичное описание полезно, если границы известного обозначены."
        action={<CreateInterpretationButton projectId={projectId} />}
      />
    );
  if (interp.isLoading || version.isLoading) return <Loader />;
  if (interp.error || !interp.data) return <ErrorState error={interp.error} onRetry={interp.refetch} />;
  if (version.error || !version.data) return <ErrorState error={version.error} onRetry={version.refetch} />;

  // Remount on version switch so the draft starts from that version.
  return (
    <EditorWorkspace
      key={version.data.id}
      projectId={projectId}
      interp={interp.data}
      interpretations={list.data?.items ?? []}
      version={version.data}
      versions={versions.data}
    />
  );
}

type LeftTab = 'editor' | 'outline' | 'versions';

interface EditorWorkspaceProps {
  projectId: string;
  interp: Interpretation;
  interpretations: Interpretation[];
  version: InterpretationVersion;
  versions?: ListResponse<InterpretationVersion>;
}

function EditorWorkspace({ projectId, interp, interpretations, version, versions }: EditorWorkspaceProps) {
  const navigate = useNavigate();
  const draft = useSpecDraft(version.spec);
  const [tab, setTab] = useState<LeftTab>('editor');
  const hypotheses = useQuery(hypothesisQueries.list(projectId));
  const isLatest = version.id === interp.latestVersionId;

  const text = useDebouncedValue(draft.text, 450);
  const parsed = (() => {
    try {
      return JSON.parse(text) as unknown;
    } catch {
      return undefined;
    }
  })();
  const validation = useQuery({
    queryKey: ['spec-validate', text],
    queryFn: () => interpretationApi.validate(parsed),
    enabled: parsed !== undefined,
    staleTime: Infinity,
  });
  const valid = validation.data?.valid ?? false;
  // Known to be broken: not merely not validated yet, so a valid spec never flashes the empty state.
  const invalid = !!draft.syntaxError || validation.data?.valid === false;

  const sessions = useQuery(sessionQueries.list(projectId, { target: true }));
  const [sessionOverride, setSessionOverride] = useState('');
  const sessionId = sessionOverride || sessions.data?.items[0]?.id || '';
  const preview = useQuery({
    queryKey: ['spec-preview', sessionId, text],
    queryFn: () => interpretationApi.preview(JSON.parse(text), sessionId),
    enabled: !!sessionId && valid,
    staleTime: Infinity,
  });
  const [activeIndex, setActiveIndex] = useState<number | null>(null);
  const messages = preview.data?.items ?? [];
  const active = messages.find((m) => m.index === activeIndex);
  const counts = messages.reduce<Record<string, number>>((acc, m) => ({ ...acc, [m.status]: (acc[m.status] ?? 0) + 1 }), {});

  return (
    <div className="flex h-full flex-col">
      <div className="flex flex-wrap items-center gap-3 border-b border-line bg-surface px-4 py-2">
        {interpretations.length > 1 ? (
          <Select
            value={interp.id}
            onChange={(e) => navigate(routes.spec(projectId, { interpretationId: e.target.value }))}
            className="w-56"
            title="Интерпретация"
          >
            {interpretations.map((i) => (
              <option key={i.id} value={i.id}>
                {i.name}
              </option>
            ))}
          </Select>
        ) : (
          <span className="font-semibold">{interp.name}</span>
        )}
        <Select
          value={version.id}
          onChange={(e) => navigate(routes.spec(projectId, { interpretationId: interp.id, versionId: e.target.value }))}
          className="w-40"
          title="Версия"
        >
          {versions?.items.map((v) => (
            <option key={v.id} value={v.id}>
              v{v.version}
              {v.id === interp.latestVersionId ? ' (актуальная)' : ''}
            </option>
          ))}
        </Select>
        <span className="font-mono text-2xs text-muted" title={`sha256 ${version.hash}`}>
          {shortHash(version.hash, 12)}
        </span>
        {draft.dirty && <Status tone="warn">есть несохранённые изменения</Status>}
        <div className="ml-auto flex items-center gap-2">
          <CreateInterpretationButton projectId={projectId} variant="ghost" label="Новая" />
          <StartRunButton projectId={projectId} defaultVersionId={version.id} variant="secondary" label="Проверить" />
          <SaveVersionButton
            projectId={projectId}
            interpretationId={interp.id}
            spec={draft.spec}
            disabled={!draft.dirty || !valid}
            onSaved={(v) => navigate(routes.spec(projectId, { interpretationId: interp.id, versionId: v.id }))}
          />
        </div>
      </div>

      {!isLatest && (
        <Note tone="warn" className="mx-4 mt-3">
          Просматривается v{version.version}; актуальная — v{interp.latestVersion}. Сохранение создаст новую версию на основе этого текста.
        </Note>
      )}

      <div className="grid min-h-0 flex-1 grid-cols-1 xl:grid-cols-[minmax(0,1.15fr)_minmax(0,1fr)]">
        <section className="flex min-h-0 flex-col border-r border-line">
          <Tabs<LeftTab>
            value={tab}
            onChange={setTab}
            items={[
              { id: 'editor', label: 'Спецификация' },
              { id: 'outline', label: 'Структура' },
              { id: 'versions', label: 'Версии', count: versions?.total },
            ]}
          />
          <div className="min-h-0 flex-1 overflow-hidden p-3">
            {tab === 'editor' && (
              <SpecEditor
                className="h-full"
                value={draft.text}
                onChange={draft.setText}
                syntaxError={draft.syntaxError}
                issues={validation.data?.issues}
              />
            )}
            {tab === 'outline' && (
              <div className="h-full overflow-y-auto pr-1">
                {draft.spec && valid ? (
                  <SpecOutline projectId={projectId} spec={draft.spec} hypotheses={hypotheses.data?.items} />
                ) : (
                  <EmptyState title="Спецификация содержит ошибки" description="Исправьте ошибки на вкладке «Спецификация»." />
                )}
              </div>
            )}
            {tab === 'versions' && (
              <ul className="h-full divide-y divide-line overflow-y-auto border border-line">
                {versions?.items.map((v) => (
                  <li key={v.id} className={cn('flex items-start gap-3 px-3 py-2.5', v.id === version.id && 'bg-selection')}>
                    <Tag>v{v.version}</Tag>
                    <div className="min-w-0 flex-1">
                      <p className="text-xs">{v.comment}</p>
                      <p className="font-mono text-2xs text-muted">
                        {formatDateTime(v.createdAt)} · {shortHash(v.hash, 16)}
                      </p>
                    </div>
                    {v.id === interp.latestVersionId && <Tag>актуальная</Tag>}
                    {v.id !== version.id && (
                      <Link to={routes.spec(projectId, { interpretationId: interp.id, versionId: v.id })} className="link text-xs">
                        открыть
                      </Link>
                    )}
                    <DownloadButton
                      href={interpretationApi.exportUrl(v.id)}
                      fileName={`${interp.name}.v${v.version}.json`}
                      variant="ghost"
                      size="xs"
                    >
                      JSON
                    </DownloadButton>
                  </li>
                ))}
              </ul>
            )}
          </div>
        </section>

        <section className="flex min-h-0 flex-col">
          <div className="flex h-9 shrink-0 items-center gap-3 border-b border-line px-3">
            <span className="text-sm font-medium">Предпросмотр</span>
            {valid && (
              <span className="flex items-center gap-3 text-xs">
                {Object.entries(counts).map(([status, n]) => (
                  <Status key={status} tone={status === 'ok' ? 'ok' : status === 'violation' ? 'danger' : 'warn'}>
                    {status} <span className="font-mono">{n}</span>
                  </Status>
                ))}
              </span>
            )}
            <Select
              value={sessionId}
              onChange={(e) => {
                setSessionOverride(e.target.value);
                setActiveIndex(null);
              }}
              className="ml-auto w-36"
              aria-label="Сессия для предпросмотра"
            >
              {sessions.data?.items.map((s) => (
                <option key={s.id} value={s.id} title={s.capture.fileName}>
                  {s.id}
                </option>
              ))}
            </Select>
          </div>
          <div className="min-h-0 flex-1 overflow-y-auto">
            {invalid && <EmptyState title="Предпросмотра нет" description="Он появится, когда спецификация станет корректной." />}
            {valid && preview.isFetching && !preview.data && <Loader />}
            {preview.error && <ErrorState error={preview.error} />}
            <MessageList messages={messages} activeIndex={activeIndex} onSelect={(m) => setActiveIndex(m.index)} />
            {active && (
              <div className="border-t border-line p-3">
                <FieldTable message={active} selection={null} link={null} onSelectRange={() => undefined} onLink={() => undefined} />
                <Link
                  to={routes.session(projectId, active.sessionId, {
                    dir: active.direction,
                    v: draft.dirty ? undefined : version.id,
                    sel: { offset: active.offset, length: active.length },
                  })}
                  className="link mt-2 inline-flex items-center gap-1.5 text-xs"
                >
                  Открыть в потоке <Arrow />
                </Link>
              </div>
            )}
          </div>
        </section>
      </div>
    </div>
  );
}
