import type { TcpSession } from '@/entities/session';
import type { StreamSegment } from '@/entities/stream';
import { CreateObservationButton } from '@/features/create-observation';
import type { Direction, SourceRef } from '@/shared/api';
import { decodeAt, formatRange, maskHasAny, rangeEnd, toAscii, toHexMasked, type ByteRange } from '@/shared/lib';
import { Note, CopyButton, EmptyState, KeyValue, Mono, SectionTitle } from '@/shared/ui';

export interface ByteInspectorProps {
  projectId: string;
  session: TcpSession;
  direction: Direction;
  bytes: Uint8Array;
  unknownMask: Uint8Array;
  segments: StreamSegment[];
  selection: ByteRange | null;
}

const PREVIEW_LIMIT = 256;

/** Decodes the selection and shows its provenance (capture → session → packets → bytes). */
export function ByteInspector({ projectId, session, direction, bytes, unknownMask, segments, selection }: ByteInspectorProps) {
  if (!selection) {
    return (
      <EmptyState
        title="Выделите байты"
        description="Значения выделения будут показаны в разных кодировках вместе с пакетами-источниками."
      />
    );
  }

  const slice = bytes.subarray(selection.offset, rangeEnd(selection));
  const sliceMask = unknownMask.subarray(selection.offset, rangeEnd(selection));
  const hasUnknown = maskHasAny(unknownMask, selection);
  const packets = segments
    .filter((s) => s.offset < rangeEnd(selection) && selection.offset < s.offset + s.length)
    .map((s) => s.packetNumber);
  const source: SourceRef = {
    captureId: session.captureId,
    captureSha256: session.capture.sha256,
    sessionId: session.id,
    direction,
    offset: selection.offset,
    length: selection.length,
    packetNumbers: packets,
  };
  const hex = toHexMasked(slice.subarray(0, PREVIEW_LIMIT), sliceMask);
  const decoded = hasUnknown ? [] : decodeAt(bytes, selection.offset, rangeEnd(selection));

  return (
    <div className="flex flex-col gap-4 p-4">
      <div className="flex items-start justify-between gap-2">
        <KeyValue
          items={[
            ['Диапазон', <Mono key="r">{formatRange(selection)}</Mono>],
            ['Пакеты', <Mono key="p">{packets.length ? `#${packets.join(', #')}` : '—'}</Mono>],
            ['Запись', <span key="c" className="text-muted">{session.capture.fileName}</span>],
          ]}
        />
        <CreateObservationButton projectId={projectId} source={source} previewHex={hex} hasUnknownBytes={hasUnknown} />
      </div>

      {hasUnknown && (
        <Note tone="warn" title="В выделении есть неизвестные байты">
          Эти байты не захвачены — значения ниже не вычисляются.
        </Note>
      )}

      <div>
        <SectionTitle actions={<CopyButton value={toHexMasked(slice, sliceMask)} label="hex" />}>Байты</SectionTitle>
        <p className="font-mono text-[12px] break-all text-fg">
          {hex}
          {slice.length > PREVIEW_LIMIT && <span className="text-subtle"> … ещё {slice.length - PREVIEW_LIMIT} Б</span>}
        </p>
        <p className="mt-1 font-mono text-[12px] break-all text-muted">{toAscii(slice.subarray(0, PREVIEW_LIMIT), '.', sliceMask)}</p>
      </div>

      {decoded.length > 0 && (
        <div>
          <SectionTitle>Значение с начала выделения</SectionTitle>
          <div className="grid grid-cols-2 gap-x-4 gap-y-0.5">
            {decoded.map((d) => (
              <div key={d.label} className="flex justify-between gap-2 border-b border-line/60 py-0.5 font-mono text-[12px]">
                <span className="text-subtle">{d.label}</span>
                <span>{d.value}</span>
              </div>
            ))}
          </div>
          <p className="mt-1.5 text-[11px] text-subtle">Показаны только интерпретации, целиком попадающие в выделение.</p>
        </div>
      )}
    </div>
  );
}
