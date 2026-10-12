import { useQuery } from '@tanstack/react-query';
import { ArrowLeft, Download, Eye, GitBranch, ListTree, PencilLine } from 'lucide-react';
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
import { SaveVersionButton, SpecEditor, useSpecDraft } from '@/features/edit-interpretation';
import { StartRunButton } from '@/features/run-verification';
import { FieldTable, MessageList } from '@/widgets/message-panel';
import { SpecOutline } from '@/widgets/spec-outline';
import type { ListResponse } from '@/shared/api';
import { routes } from '@/shared/config';
import { cn, formatDateTime, shortHash, useDebouncedValue, useRequiredParams } from '@/shared/lib';
import { Badge, Callout, DownloadButton, EmptyState, ErrorState, Loader, Select, Tabs } from '@/shared/ui';

export function InterpretationEditorPage() {
  const { projectId, interpretationId } = useRequiredParams('projectId', 'interpretationId');
  const [params] = useSearchParams();
  const interp = useQuery(interpretationQueries.detail(interpretationId));
  const versions = useQuery(interpretationQueries.versions(interpretationId));
  const versionId = params.get('v') ?? interp.data?.latestVersionId;
  const version = useQuery({ ...interpretationQueries.version(versionId ?? ''), enabled: !!versionId });

  if (interp.isLoading || version.isLoading) return <Loader />;
  if (interp.error || !interp.data) return <ErrorState error={interp.error} onRetry={interp.refetch} />;
  if (version.error || !version.data) return <ErrorState error={version.error} onRetry={version.refetch} />;

  // Remount on version switch so the draft starts from that version.
  return (
    <EditorWorkspace
      key={version.data.id}
      projectId={projectId}
      interp={interp.data}
      version={version.data}
      versions={versions.data}
    />
  );
}

type LeftTab = 'editor' | 'outline' | 'versions';

interface EditorWorkspaceProps {
  projectId: string;
  interp: Interpretation;
  version: InterpretationVersion;
  versions?: ListResponse<InterpretationVersion>;
}

function EditorWorkspace({ projectId, interp, version, versions }: EditorWorkspaceProps) {
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
      <div className="flex flex-wrap items-center gap-3 border-b border-line bg-surface px-4 py-2.5">
        <Link to={routes.interpretations(projectId)} className="text-muted hover:text-fg">
          <ArrowLeft size={16} />
        </Link>
        <span className="font-semibold">{interp.name}</span>
        <Select
          value={version.id}
          onChange={(e) => navigate(routes.interpretation(projectId, interp.id, e.target.value))}
          className="w-40"
        >
          {versions?.items.map((v) => (
            <option key={v.id} value={v.id}>
              v{v.version}
              {v.id === interp.latestVersionId ? ' (актуальная)' : ''}
            </option>
          ))}
        </Select>
        <span className="font-mono text-[11px] text-subtle" title={version.hash}>
          sha256 {shortHash(version.hash, 12)}
        </span>
        {draft.dirty && <Badge tone="warning">есть несохранённые изменения</Badge>}
        <div className="ml-auto flex items-center gap-2">
          <DownloadButton href={interpretationApi.exportUrl(version.id)} fileName={`${interp.name}.v${version.version}.json`}>
            <Download size={14} /> JSON
          </DownloadButton>
          <StartRunButton projectId={projectId} defaultVersionId={version.id} variant="secondary" label="Проверить" />
          <SaveVersionButton
            projectId={projectId}
            interpretationId={interp.id}
            spec={draft.spec}
            disabled={!draft.dirty || !valid}
            onSaved={(v) => navigate(routes.interpretation(projectId, interp.id, v.id))}
          />
        </div>
      </div>

      {!isLatest && (
        <Callout tone="warning" className="mx-4 mt-3">
          Просматривается v{version.version}; актуальная — v{interp.latestVersion}. Сохранение создаст новую версию на основе этого текста.
        </Callout>
      )}

      <div className="grid min-h-0 flex-1 grid-cols-1 xl:grid-cols-[minmax(0,1.15fr)_minmax(0,1fr)]">
        <section className="flex min-h-0 flex-col border-r border-line">
          <Tabs<LeftTab>
            value={tab}
            onChange={setTab}
            items={[
              { id: 'editor', label: 'Спецификация', icon: <PencilLine size={13} /> },
              { id: 'outline', label: 'Структура', icon: <ListTree size={13} /> },
              { id: 'versions', label: 'Версии', icon: <GitBranch size={13} />, badge: versions?.total },
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
              <ul className="h-full divide-y divide-line overflow-y-auto rounded-md border border-line">
                {versions?.items.map((v) => (
                  <li key={v.id} className={cn('flex items-start gap-3 px-3 py-2.5', v.id === version.id && 'bg-accent/8')}>
                    <span className="rounded bg-surface-3 px-1.5 font-mono text-[12px]">v{v.version}</span>
                    <div className="min-w-0 flex-1">
                      <p className="text-[12.5px]">{v.comment}</p>
                      <p className="font-mono text-[11px] text-subtle">
                        {formatDateTime(v.createdAt)} · {shortHash(v.hash, 16)}
                      </p>
                    </div>
                    {v.id === interp.latestVersionId && <Badge tone="success">актуальная</Badge>}
                    {v.id !== version.id && (
                      <Link to={routes.interpretation(projectId, interp.id, v.id)} className="text-xs text-info hover:underline">
                        открыть
                      </Link>
                    )}
                  </li>
                ))}
              </ul>
            )}
          </div>
        </section>

        <section className="flex min-h-0 flex-col">
          <div className="flex items-center gap-2 border-b border-line px-3 py-2">
            <Eye size={14} className="text-muted" />
            <span className="text-[12.5px] font-medium">Предпросмотр черновика</span>
            <Select
              value={sessionId}
              onChange={(e) => {
                setSessionOverride(e.target.value);
                setActiveIndex(null);
              }}
              className="ml-auto w-64"
            >
              {sessions.data?.items.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.id} · {s.capture.fileName}
                </option>
              ))}
            </Select>
          </div>
          <div className="flex flex-wrap gap-1.5 border-b border-line px-3 py-2">
            {Object.entries(counts).map(([status, n]) => (
              <Badge key={status} tone={status === 'ok' ? 'success' : status === 'violation' ? 'danger' : 'warning'}>
                {status}: {n}
              </Badge>
            ))}
            {!valid && <span className="text-xs text-muted">Предпросмотр появится, когда спецификация станет корректной.</span>}
          </div>
          <div className="min-h-0 flex-1 overflow-y-auto">
            {preview.isFetching && !preview.data && <Loader />}
            {preview.error && <ErrorState error={preview.error} />}
            <MessageList messages={messages} activeIndex={activeIndex} onSelect={(m) => setActiveIndex(m.index)} />
            {active && (
              <div className="border-t border-line p-3">
                <FieldTable message={active} onSelectRange={() => undefined} />
                <Link
                  to={routes.session(projectId, active.sessionId, {
                    dir: active.direction,
                    v: draft.dirty ? undefined : version.id,
                    sel: { offset: active.offset, length: active.length },
                  })}
                  className="mt-2 inline-block text-xs text-info hover:underline"
                >
                  Открыть в потоке →
                </Link>
              </div>
            )}
          </div>
        </section>
      </div>
    </div>
  );
}
