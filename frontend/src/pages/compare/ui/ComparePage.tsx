import { useQuery } from '@tanstack/react-query';
import { useMemo, useState } from 'react';
import { actionQueries, numericParams, type ActionEntry } from '@/entities/action';
import { interpretationQueries } from '@/entities/interpretation';
import { messageQueries } from '@/entities/message';
import { CreateHypothesisButton } from '@/features/manage-hypothesis';
import { CandidateList, CompareMatrix, type CompareRowData } from '@/widgets/compare-matrix';
import { routes } from '@/shared/config';
import {
  base64ToBytes,
  columnStats,
  ENCODING_SIZE,
  findFieldCandidates,
  useRequiredParams,
  type FieldCandidate,
} from '@/shared/lib';
import { Pane, PaneHeader, EmptyState, ErrorState, Field, Loader, Page, PageHeader, Select } from '@/shared/ui';

/** Candidate value paths of an action: params.* and observed.* */
function valuePaths(action: ActionEntry | undefined): Record<string, number> {
  if (!action) return {};
  const out: Record<string, number> = {};
  for (const [k, v] of Object.entries(numericParams(action.params))) out[`params.${k}`] = v;
  for (const [k, v] of Object.entries(numericParams(action.observed))) out[`observed.${k}`] = v;
  return out;
}

export function ComparePage() {
  const { projectId } = useRequiredParams('projectId');
  const interpretations = useQuery(interpretationQueries.list(projectId));
  const [versionOverride, setVersionOverride] = useState('');
  const versionId = versionOverride || interpretations.data?.items[0]?.latestVersionId || '';
  const version = useQuery({ ...interpretationQueries.version(versionId), enabled: !!versionId });
  const [ruleOverride, setRuleOverride] = useState('');
  // Rules tied to experiment conditions (assertions) are the natural starting point for comparison.
  const spec = version.data?.spec;
  const ruleId = ruleOverride || spec?.assertions?.[0]?.rule || spec?.messages[0]?.id || '';
  const assertedPaths = spec?.assertions?.filter((a) => a.rule === ruleId).flatMap((a) => a.checks.map((c) => c.equals)) ?? [];
  const [paramPath, setParamPath] = useState('');
  const [picked, setPicked] = useState<FieldCandidate | null>(null);

  const messages = useQuery({ ...messageQueries.byProject(projectId, { versionId, ruleId }), enabled: !!versionId && !!ruleId });
  const actions = useQuery(actionQueries.list(projectId));

  const actionBySession = useMemo(() => {
    const map = new Map<string, ActionEntry>();
    for (const a of actions.data?.items ?? []) for (const l of a.links) map.set(l.sessionId, a);
    return map;
  }, [actions.data]);

  const items = useMemo(() => messages.data?.items ?? [], [messages.data]);
  const paths = useMemo(() => {
    const set = new Set<string>();
    for (const m of items) for (const k of Object.keys(valuePaths(actionBySession.get(m.sessionId)))) set.add(k);
    return [...set].sort();
  }, [items, actionBySession]);
  const activePath =
    paramPath && paths.includes(paramPath)
      ? paramPath
      : ([...assertedPaths].reverse().find((p) => paths.includes(p)) ?? paths[0] ?? '');

  const rows: CompareRowData[] = useMemo(
    () =>
      items.map((m) => {
        const action = actionBySession.get(m.sessionId);
        return {
          key: `${m.sessionId}:${m.index}`,
          label: action?.label ?? `${m.sessionId} (действие не связано)`,
          sublabel: `${m.sessionId} · #${m.index} · ${m.length} Б`,
          bytes: base64ToBytes(m.data),
          param: activePath ? (valuePaths(action)[activePath] ?? null) : null,
          href: routes.session(projectId, m.sessionId, { dir: m.direction, v: versionId, sel: { offset: m.offset, length: m.length } }),
        };
      }),
    [items, actionBySession, activePath, projectId, versionId],
  );

  const stats = useMemo(() => columnStats(rows), [rows]);
  const search = useMemo(() => findFieldCandidates(rows), [rows]);

  return (
    <Page className="max-w-none">
      <PageHeader
        title="Сравнение примеров"
      />
      <div className="mb-4 flex flex-wrap items-end gap-3">
        <Field label="Интерпретация (кадрирование)" className="w-64">
          <Select
            value={versionId}
            onChange={(e) => {
              setVersionOverride(e.target.value);
              setRuleOverride('');
              setPicked(null);
            }}
          >
            {interpretations.data?.items.map((i) => (
              <option key={i.id} value={i.latestVersionId}>
                {i.name} v{i.latestVersion}
              </option>
            ))}
          </Select>
        </Field>
        <Field label="Тип сообщения" className="w-64">
          <Select
            value={ruleId}
            onChange={(e) => {
              setRuleOverride(e.target.value);
              setPicked(null);
            }}
          >
            {spec?.messages.map((m) => (
              <option key={m.id} value={m.id}>
                {m.id}
              </option>
            ))}
          </Select>
        </Field>
        <Field label="Сопоставить с" className="w-56">
          <Select
            value={activePath}
            onChange={(e) => {
              setParamPath(e.target.value);
              setPicked(null);
            }}
          >
            {paths.length === 0 && <option value="">нет числовых параметров</option>}
            {paths.map((p) => (
              <option key={p} value={p}>
                {p}
              </option>
            ))}
          </Select>
        </Field>
        <span className="mb-2 text-xs text-muted">{rows.length} сообщений</span>
      </div>

      {!versionId && !interpretations.isLoading && (
        <EmptyState title="Нужна интерпретация" description="Сравнение работает с сообщениями, выделенными правилом кадрирования." />
      )}
      {messages.isLoading && <Loader />}
      {messages.error && <ErrorState error={messages.error} onRetry={messages.refetch} />}

      {rows.length > 0 && (
        <div className="grid gap-4 xl:grid-cols-[1fr_380px]">
          <div className="min-w-0">
            <CompareMatrix
              rows={rows}
              stats={stats}
              paramLabel={activePath.split('.')[1] ?? 'параметр'}
              highlight={picked ? { offset: picked.offset, length: ENCODING_SIZE[picked.encoding] } : null}
            />
            <p className="mt-2 text-[11.5px] text-subtle">
              Цвет номера столбца: <span className="text-subtle">константа</span> · <span className="text-fg">различается</span> ·{' '}
              <span className="text-info">есть не во всех сообщениях</span>
            </p>
          </div>
          <Pane className="self-start">
            <PaneHeader title="Кандидаты в поля" />
            <div className="p-3">
              <CandidateList
                search={search}
                rowLabels={rows.map((r) => r.label)}
                active={picked}
                onPick={setPicked}
                renderAction={(c) => (
                  <CreateHypothesisButton
                    projectId={projectId}
                    size="xs"
                    variant="ghost"
                    label="Гипотеза"
                    defaults={{
                      title: `${ruleId}: байты @${c.offset} (${c.encoding}${c.scale !== 1 ? ` ÷${c.scale}` : ''}) = ${activePath}`,
                      statement: `В сообщениях ${ruleId} значение ${c.encoding} по смещению ${c.offset}${c.scale !== 1 ? `, делённое на ${c.scale},` : ''} равно ${activePath} связанного действия. Совпало в ${c.matches} из ${c.total} примеров.`,
                      ruleRefs: [`${ruleId}.@${c.offset}`],
                    }}
                  />
                )}
              />
            </div>
          </Pane>
        </div>
      )}
    </Page>
  );
}
