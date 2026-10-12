import { ArrowLeftIcon } from '@phosphor-icons/react';
import { useQuery } from '@tanstack/react-query';
import { useEffect, useMemo, useState } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router';
import { DiagnosticList } from '@/entities/capture';
import { interpretationQueries } from '@/entities/interpretation';
import { messageQueries, type Message } from '@/entities/message';
import { DIRECTION_LABEL, EndpointPair, SessionAnomalyMark, SessionStatusMark, sessionQueries } from '@/entities/session';
import { streamQueries, type Packet } from '@/entities/stream';
import { ValueSearch } from '@/features/value-search';
import { ByteInspector } from '@/widgets/byte-inspector';
import { HexViewer, type HexHighlight } from '@/widgets/hex-viewer';
import { FieldTable, fieldColor, MessageList } from '@/widgets/message-panel';
import { PacketTable } from '@/widgets/packet-table';
import { useContextKeys } from '@/widgets/soft-keys';
import type { Direction } from '@/shared/api';
import { routes } from '@/shared/config';
import { buildMask, cn, parseRange, serializeRange, useRequiredParams, type ByteRange } from '@/shared/lib';
import { EmptyState, ErrorState, Loader, Note, Select, Status, Tabs } from '@/shared/ui';

type PanelTab = 'inspector' | 'messages' | 'packets' | 'search';

const NO_VERSION = 'none';

export function SessionWorkspacePage() {
  const { projectId, sessionId } = useRequiredParams('projectId', 'sessionId');
  const [params, setParams] = useSearchParams();
  const navigate = useNavigate();
  const dir: Direction = params.get('dir') === 's2c' ? 's2c' : 'c2s';
  const selection = parseRange(params.get('sel'));

  const session = useQuery(sessionQueries.detail(sessionId));
  const meta = useQuery(streamQueries.meta(sessionId, dir));
  const bytes = useQuery(streamQueries.bytes(sessionId, dir));
  const packets = useQuery(streamQueries.packets(sessionId));
  const interpretations = useQuery(interpretationQueries.list(projectId));

  const versionId = params.get('v') ?? interpretations.data?.items[0]?.latestVersionId ?? NO_VERSION;
  const hasVersion = versionId !== NO_VERSION;
  const version = useQuery({ ...interpretationQueries.version(versionId), enabled: hasVersion });
  const messages = useQuery({ ...messageQueries.bySession(sessionId, versionId), enabled: hasVersion });

  const [tab, setTab] = useState<PanelTab>(selection ? 'inspector' : 'messages');
  const [activeIndex, setActiveIndex] = useState<number | null>(null);
  const [hits, setHits] = useState<ByteRange[]>([]);
  const [scrollTo, setScrollTo] = useState<{ offset: number; nonce: number }>();

  const update = (patch: Record<string, string | null>) => {
    const next = new URLSearchParams(params);
    for (const [k, v] of Object.entries(patch)) {
      if (v === null) next.delete(k);
      else next.set(k, v);
    }
    setParams(next, { replace: true });
  };

  const select = (range: ByteRange | null, opts: { dir?: Direction; scroll?: boolean } = {}) => {
    const targetDir = opts.dir ?? dir;
    if (targetDir !== dir) setHits([]);
    update({ dir: targetDir, sel: range ? serializeRange(range) : null });
    if (range && opts.scroll) setScrollTo({ offset: range.offset, nonce: Date.now() });
  };

  // Deep links (from findings and observations) scroll to the referenced bytes.
  const initialSel = params.get('sel');
  useEffect(() => {
    const r = parseRange(initialSel);
    if (r && bytes.data) setScrollTo({ offset: r.offset, nonce: 1 });
    // eslint-disable-next-line react-hooks/exhaustive-deps -- only once bytes arrive
  }, [bytes.data]);

  const unknownMask = useMemo(() => buildMask(bytes.data?.length ?? 0, meta.data?.gaps ?? []), [bytes.data, meta.data]);

  const allMessages = useMemo(() => messages.data?.items ?? [], [messages.data]);
  const messageStarts = useMemo(() => allMessages.filter((m) => m.direction === dir).map((m) => m.offset), [allMessages, dir]);
  const active = allMessages.find((m) => m.index === activeIndex && m.direction === dir);

  const highlights = useMemo(() => {
    const list: HexHighlight[] = hits.map((h) => ({ ...h, color: '#facc15', label: 'совпадение поиска' }));
    active?.fields.forEach((f, i) =>
      list.push({
        offset: active.offset + f.offset,
        length: f.length,
        color: fieldColor(i),
        label: `${active.ruleId}.${f.name} = ${f.display}`,
      }),
    );
    return list;
  }, [hits, active]);

  // Field colours stay visible: the message is scrolled to, not selected as a whole.
  const onMessage = (m: Message) => {
    setActiveIndex(m.index);
    if (m.direction !== dir) setHits([]);
    update({ dir: m.direction, sel: null });
    setScrollTo({ offset: m.offset, nonce: Date.now() });
  };

  const onPacket = (p: Packet) => {
    if (p.streamOffset === null) return;
    select({ offset: p.streamOffset, length: p.payloadLength }, { dir: p.direction, scroll: true });
    setTab('inspector');
  };

  useContextKeys([
    { key: '/', label: 'поиск', description: 'Поиск значения в потоке', onPress: () => setTab('search') },
    {
      key: 'D',
      label: 'направление',
      description: 'Переключить направление потока',
      onPress: () => select(null, { dir: dir === 'c2s' ? 's2c' : 'c2s' }),
    },
    { key: 'C', label: 'сравнить', description: 'Сравнение примеров', onPress: () => navigate(routes.compare(projectId)) },
  ]);

  if (session.isLoading) return <Loader />;
  if (session.error || !session.data) return <ErrorState error={session.error} onRetry={session.refetch} />;
  const s = session.data;
  const counts = allMessages.reduce<Record<string, number>>((acc, m) => ({ ...acc, [m.status]: (acc[m.status] ?? 0) + 1 }), {});

  return (
    <div className="flex h-full flex-col">
      <div className="flex flex-wrap items-center gap-3 border-b border-line bg-surface px-4 py-2.5">
        <Link to={routes.sessions(projectId)} className="text-muted hover:text-fg" title="К списку сессий">
          <ArrowLeftIcon size={16} />
        </Link>
        <span className="font-mono text-[13px] font-semibold">{s.id}</span>
        <EndpointPair client={s.client} server={s.server} />
        <SessionStatusMark status={s.status} />
        <SessionAnomalyMark flags={s.flags} roles={s.roles} />

        <div className="ml-auto flex items-center gap-2">
          <div className="flex border border-line bg-bg p-0.5 text-xs">
            {(['c2s', 's2c'] as const).map((d) => (
              <button
                key={d}
                type="button"
                onClick={() => select(null, { dir: d })}
                className={cn('px-2.5 py-1', dir === d ? 'bg-surface-2 text-fg' : 'text-muted hover:text-fg')}
              >
                {DIRECTION_LABEL[d]}
                <span className="ml-1.5 font-mono text-[10.5px] text-subtle">{s.bytes[d]} Б</span>
              </button>
            ))}
          </div>
          <Select
            value={versionId}
            onChange={(e) => {
              setActiveIndex(null);
              update({ v: e.target.value });
            }}
            className="w-64"
            title="Интерпретация для кадрирования и разбора"
          >
            <option value={NO_VERSION}>Без интерпретации (сырой поток)</option>
            {interpretations.data?.items.map((i) => (
              <option key={i.id} value={i.latestVersionId}>
                {i.name} v{i.latestVersion} · актуальная
              </option>
            ))}
            {version.data && !interpretations.data?.items.some((i) => i.latestVersionId === versionId) && (
              <option value={versionId}>v{version.data.version} (устаревшая)</option>
            )}
          </Select>
        </div>
      </div>

      {s.diagnostics.length > 0 && (
        <Note tone="warn" className="mx-4 mt-3" title="Неполные или неоднозначные данные">
          <DiagnosticList items={s.diagnostics} className="mt-1" />
        </Note>
      )}

      <div className="flex min-h-0 flex-1">
        <div className="flex min-w-0 flex-1 flex-col">
          {meta.data && !meta.data.startKnown && (
            <p className="border-b border-line px-4 py-1.5 text-xs text-warn">
              Начало потока не захвачено: смещение 0 соответствует первому полученному байту, а не началу соединения.
            </p>
          )}
          {bytes.isLoading || meta.isLoading ? (
            <Loader />
          ) : bytes.error || meta.error ? (
            <ErrorState error={bytes.error ?? meta.error} />
          ) : (
            <HexViewer
              className="min-h-0 flex-1"
              bytes={bytes.data!}
              unknownMask={unknownMask}
              segments={meta.data!.segments}
              messageStarts={messageStarts}
              highlights={highlights}
              selection={selection}
              onSelectionChange={(r) => {
                select(r);
                setTab('inspector');
              }}
              scrollTo={scrollTo}
            />
          )}
          <div className="flex flex-wrap items-center gap-3 border-t border-line bg-surface px-4 py-1.5 text-[11px] text-muted">
            <Legend className="bg-surface-2" label="чередование TCP-сегментов" />
            <Legend className="shadow-[inset_2px_0_0_var(--color-line-strong)]" label="начало сообщения" />
            <Legend className="bg-hatch" label="неизвестные байты" />
            <Legend className="bg-selection" label="выделение" />
          </div>
        </div>

        <aside className="flex w-[480px] shrink-0 flex-col border-l border-line bg-surface">
          <Tabs<PanelTab>
            value={tab}
            onChange={setTab}
            items={[
              { id: 'inspector', label: 'Инспектор' },
              { id: 'messages', label: 'Сообщения', count: hasVersion ? allMessages.length : undefined },
              { id: 'packets', label: 'Пакеты', count: packets.data?.total },
              { id: 'search', label: 'Поиск', count: hits.length || undefined },
            ]}
          />
          <div className="min-h-0 flex-1 overflow-y-auto">
            {tab === 'inspector' && bytes.data && meta.data && (
              <ByteInspector
                projectId={projectId}
                session={s}
                direction={dir}
                bytes={bytes.data}
                unknownMask={unknownMask}
                segments={meta.data.segments}
                selection={selection}
              />
            )}

            {tab === 'messages' &&
              (!hasVersion ? (
                <EmptyState
                  title="Сообщения не выделены"
                  description="Граница TCP-пакета не определяет границу сообщения. Выберите интерпретацию, чтобы применить правила кадрирования."
                  action={
                    <Link to={routes.spec(projectId)} className="text-xs link">
                      К интерпретациям
                    </Link>
                  }
                />
              ) : messages.isLoading ? (
                <Loader />
              ) : messages.error ? (
                <ErrorState error={messages.error} onRetry={messages.refetch} />
              ) : (
                <div className="flex flex-col">
                  <div className="flex flex-wrap gap-1.5 border-b border-line px-3 py-2">
                    {Object.entries(counts).map(([status, n]) => (
                      <Status key={status} tone={status === 'ok' ? 'ok' : status === 'violation' ? 'danger' : 'warn'} className="text-xs">
                        {status}: {n}
                      </Status>
                    ))}
                    {version.data && <span className="ml-auto font-mono text-[11px] text-subtle">v{version.data.version}</span>}
                  </div>
                  <MessageList messages={allMessages} activeIndex={activeIndex} onSelect={onMessage} />
                  {active && (
                    <div className="border-t border-line p-3">
                      <p className="mb-2 text-[12px] font-semibold">
                        #{active.index} {active.ruleTitle ?? active.ruleId}
                      </p>
                      <FieldTable
                        message={active}
                        onSelectRange={(o, len) => select({ offset: active.offset + o, length: len }, { scroll: true })}
                      />
                    </div>
                  )}
                </div>
              ))}

            {tab === 'packets' && packets.data && <PacketTable packets={packets.data.items} onSelect={onPacket} />}

            {tab === 'search' && bytes.data && (
              <div className="p-4">
                <ValueSearch
                  bytes={bytes.data}
                  unknownMask={unknownMask}
                  onHits={setHits}
                  onSelect={(r) => select(r, { scroll: true })}
                />
              </div>
            )}
          </div>
        </aside>
      </div>
    </div>
  );
}

function Legend({ className, label }: { className: string; label: string }) {
  return (
    <span className="inline-flex items-center gap-1.5">
      <span className={cn('inline-block h-3 w-4 border border-line', className)} />
      {label}
    </span>
  );
}
