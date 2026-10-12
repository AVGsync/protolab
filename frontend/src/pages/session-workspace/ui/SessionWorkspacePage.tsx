import { useQuery } from '@tanstack/react-query';
import { useEffect, useMemo, useRef, useState } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router';
import { interpretationQueries } from '@/entities/interpretation';
import {
  fieldRange,
  findFieldAt,
  findMessageAt,
  messageQueries,
  sortByOffset,
  type FieldLink,
  type Message,
} from '@/entities/message';
import { sessionQueries } from '@/entities/session';
import { streamQueries, type Packet } from '@/entities/stream';
import { StartRunDialog } from '@/features/run-verification';
import { ValueSearch } from '@/features/value-search';
import { AnnotationLanes } from '@/widgets/annotation-lanes';
import { ByteReadout, describeSelection } from '@/widgets/byte-readout';
import { HexViewer } from '@/widgets/hex-viewer';
import { FieldTable, MessageList } from '@/widgets/message-panel';
import { PacketTable } from '@/widgets/packet-table';
import { FooterReadout, useContextKeys } from '@/widgets/soft-keys';
import type { Direction } from '@/shared/api';
import { routes } from '@/shared/config';
import { buildMask, parseRange, serializeRange, useRequiredParams, type ByteRange } from '@/shared/lib';
import { EmptyState, ErrorState, Loader, Status, Tabs } from '@/shared/ui';
import { ContextBar, NO_VERSION } from './ContextBar';

type PaneTab = 'messages' | 'packets' | 'search';

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

  const [tab, setTab] = useState<PaneTab>('messages');
  const [activeIndex, setActiveIndex] = useState<number | null>(null);
  const [hits, setHits] = useState<ByteRange[]>([]);
  const [scrollTo, setScrollTo] = useState<{ offset: number; nonce: number }>();
  const [hover, setHover] = useState<number | null>(null);
  const [rowLink, setRowLink] = useState<FieldLink | null>(null);
  const [runOpen, setRunOpen] = useState(false);
  const versionSelect = useRef<HTMLSelectElement>(null);

  const update = (patch: Record<string, string | null>) => {
    const next = new URLSearchParams(params);
    for (const [k, v] of Object.entries(patch)) {
      if (v === null) next.delete(k);
      else next.set(k, v);
    }
    setParams(next, { replace: true });
  };

  const unknownMask = useMemo(() => buildMask(bytes.data?.length ?? 0, meta.data?.gaps ?? []), [bytes.data, meta.data]);
  const allMessages = useMemo(() => messages.data?.items ?? [], [messages.data]);
  const streamMessages = useMemo(() => sortByOffset(allMessages.filter((m) => m.direction === dir)), [allMessages, dir]);

  // Bytes and messages are linked both ways: a click in the stream makes the message under it the active one.
  const select = (range: ByteRange | null, opts: { dir?: Direction; scroll?: boolean } = {}) => {
    const targetDir = opts.dir ?? dir;
    if (targetDir !== dir) setHits([]);
    update({ dir: targetDir, sel: range ? serializeRange(range) : null });
    if (range && opts.scroll) setScrollTo({ offset: range.offset, nonce: Date.now() });
    if (range && targetDir === dir) {
      const m = findMessageAt(streamMessages, range.offset);
      if (m) setActiveIndex(m.index);
    }
  };

  // Deep links (from findings and observations) scroll to the referenced bytes once the stream is there.
  const initialSel = params.get('sel');
  useEffect(() => {
    const r = parseRange(initialSel);
    if (r && bytes.data) setScrollTo({ offset: r.offset, nonce: 1 });
    // eslint-disable-next-line react-hooks/exhaustive-deps -- only once bytes arrive
  }, [bytes.data]);

  // The active message drives the lanes; before anything is chosen, the first message of the stream.
  const active = streamMessages.find((m) => m.index === activeIndex) ?? streamMessages[0];

  const hoverLink = useMemo<FieldLink | null>(() => {
    if (hover === null) return null;
    const m = findMessageAt(streamMessages, hover);
    const f = m && findFieldAt(m, hover - m.offset);
    return m && f ? { messageIndex: m.index, path: f.path, range: fieldRange(m, f) } : null;
  }, [hover, streamMessages]);
  const link = hoverLink ?? rowLink;
  const cursor = hover ?? selection?.offset ?? null;

  const details = useMemo(
    () =>
      selection && session.data && bytes.data && meta.data
        ? describeSelection(session.data, dir, bytes.data, unknownMask, meta.data.segments, selection)
        : null,
    // eslint-disable-next-line react-hooks/exhaustive-deps -- the selection is a new object on every render; its parts decide
    [selection?.offset, selection?.length, session.data, dir, bytes.data, meta.data, unknownMask],
  );

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
  };

  useContextKeys([
    { key: '/', label: 'поиск', description: 'Поиск значения в потоке', onPress: () => setTab('search') },
    {
      key: 'D',
      label: 'направление',
      description: 'Переключить направление потока',
      onPress: () => select(null, { dir: dir === 'c2s' ? 's2c' : 'c2s' }),
    },
    {
      key: 'V',
      label: 'версия',
      description: 'Выбрать интерпретацию для разбора потока',
      onPress: () => {
        const el = versionSelect.current;
        el?.focus();
        el?.showPicker?.();
      },
    },
    { key: 'C', label: 'сравнить', description: 'Сравнение примеров', onPress: () => navigate(routes.compare(projectId)) },
    {
      key: 'R',
      label: 'проверить',
      description: 'Проверить интерпретацию на корпусе',
      disabled: !hasVersion,
      onPress: () => hasVersion && setRunOpen(true),
    },
  ]);

  if (session.isLoading) return <Loader />;
  if (session.error || !session.data) return <ErrorState error={session.error} onRetry={session.refetch} />;
  const s = session.data;
  const latest = interpretations.data?.items.find((i) => i.latestVersionId === versionId);
  const staleVersion = hasVersion && !latest && version.data ? version.data.version : undefined;

  return (
    <div className="flex h-full min-h-0 flex-col">
      <ContextBar
        projectId={projectId}
        session={s}
        direction={dir}
        onDirection={(d) => select(null, { dir: d })}
        interpretations={interpretations.data?.items ?? []}
        versionId={versionId}
        staleVersion={staleVersion}
        onVersion={(v) => {
          setActiveIndex(null);
          update({ v });
        }}
        versionRef={versionSelect}
        selection={details}
      />

      <div className="flex min-h-0 flex-1">
        <div className="flex min-w-0 flex-1 flex-col border-r border-line">
          {active && version.data && bytes.data && meta.data && (
            <AnnotationLanes
              message={active}
              segments={meta.data.segments}
              gaps={meta.data.gaps}
              framing={version.data.spec.framing}
              channel={dir}
              selection={selection}
              link={link}
              onSelect={(r) => select(r, { scroll: true })}
              onLink={setRowLink}
            />
          )}
          {meta.data && !meta.data.startKnown && (
            <p className="border-b border-line px-4 py-1.5 text-xs text-muted">
              <Status tone="warn">Начало потока не захвачено: смещение 0 это первый полученный байт, а не начало соединения.</Status>
            </p>
          )}
          {bytes.isLoading || meta.isLoading ? (
            <Loader />
          ) : bytes.error || meta.error || !bytes.data || !meta.data ? (
            <ErrorState error={bytes.error ?? meta.error} />
          ) : (
            <HexViewer
              className="min-h-0 flex-1"
              bytes={bytes.data}
              unknownMask={unknownMask}
              gaps={meta.data.gaps}
              segments={meta.data.segments}
              messages={streamMessages}
              activeMessage={active?.index}
              hits={hits}
              selection={selection}
              link={link?.range}
              cursor={cursor}
              channel={dir}
              onSelectionChange={(r) => select(r)}
              onCursorChange={setHover}
              scrollTo={scrollTo}
            />
          )}
        </div>

        <aside className="flex w-[420px] shrink-0 flex-col bg-surface">
          <Tabs<PaneTab>
            value={tab}
            onChange={setTab}
            items={[
              { id: 'messages', label: 'Сообщения', count: hasVersion ? allMessages.length : undefined },
              { id: 'packets', label: 'Пакеты', count: packets.data?.total },
              { id: 'search', label: 'Поиск', count: hits.length || undefined },
            ]}
          />
          <div className="flex min-h-0 flex-1 flex-col">
            {tab === 'messages' &&
              (!hasVersion ? (
                <EmptyState
                  title="Сообщения не выделены"
                  description="Граница TCP-пакета не определяет границу сообщения. Выберите интерпретацию для кадрирования."
                  action={
                    <Link to={routes.spec(projectId)} className="link text-xs">
                      К спецификации
                    </Link>
                  }
                />
              ) : messages.isLoading ? (
                <Loader />
              ) : messages.error ? (
                <ErrorState error={messages.error} onRetry={messages.refetch} />
              ) : (
                <>
                  <MessageList
                    messages={allMessages}
                    activeIndex={active?.index ?? null}
                    onSelect={onMessage}
                    className="max-h-[40vh] shrink-0 border-b border-line"
                  />
                  {active && (
                    <div className="min-h-0 flex-1 overflow-y-auto px-4 py-3">
                      <h4 className="mb-1.5 text-sm font-semibold">
                        Поля <span className="font-mono font-normal text-muted">#{active.index}</span>
                      </h4>
                      <FieldTable message={active} selection={selection} link={link} onSelectRange={(r) => select(r, { scroll: true })} onLink={setRowLink} />
                    </div>
                  )}
                </>
              ))}

            {tab === 'packets' && packets.data && (
              <div className="min-h-0 flex-1 overflow-y-auto">
                <PacketTable packets={packets.data.items} onSelect={onPacket} />
              </div>
            )}

            {tab === 'search' && bytes.data && (
              <div className="flex min-h-0 flex-1 flex-col overflow-y-auto">
                <ValueSearch bytes={bytes.data} unknownMask={unknownMask} onHits={setHits} onSelect={(r) => select(r, { scroll: true })} />
              </div>
            )}
          </div>
        </aside>
      </div>

      {bytes.data && meta.data && (
        <FooterReadout>
          <ByteReadout bytes={bytes.data} unknownMask={unknownMask} segments={meta.data.segments} selection={selection} cursor={cursor} />
        </FooterReadout>
      )}
      <StartRunDialog projectId={projectId} open={runOpen} onClose={() => setRunOpen(false)} defaultVersionId={hasVersion ? versionId : undefined} />
    </div>
  );
}
