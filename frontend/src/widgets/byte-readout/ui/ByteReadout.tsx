import { useEffect, useState } from 'react';
import { createPortal } from 'react-dom';
import type { StreamSegment } from '@/entities/stream';
import {
  cn,
  decodeAt,
  formatOffset,
  formatRange,
  toAscii,
  toHexMasked,
  type ByteRange,
  type DecodedValue,
} from '@/shared/lib';
import { CopyButton } from '@/shared/ui';
import { packetsOf } from '../model/selection';

export interface ByteReadoutProps {
  bytes: Uint8Array;
  unknownMask: Uint8Array;
  segments: StreamSegment[];
  selection: ByteRange | null;
  /** The byte under the pointer (or the selection start). */
  cursor: number | null;
}

/** How many bytes the values at the cursor may read when nothing is selected. */
const FREE_WINDOW = 4;
const PREVIEW_LIMIT = 256;
const SUMMARY = ['u16le', 'u16be', 'i16le'];
const SUMMARY_SMALL = ['u8', 'i8'];

function valuesFor(bytes: Uint8Array, unknownMask: Uint8Array, selection: ByteRange | null, cursor: number | null): DecodedValue[] {
  const at = selection?.offset ?? cursor;
  if (at === null) return [];
  const limit = selection ? selection.offset + selection.length : at + FREE_WINDOW;
  return decodeAt(bytes, at, limit, unknownMask);
}

/**
 * Live measurement of the cursor: offset, selection length and the usual integers. Meant for the soft-key
 * footer (wrap it in FooterReadout). A click opens the converter with every interpretation of the selected bytes.
 */
export function ByteReadout({ bytes, unknownMask, segments, selection, cursor }: ByteReadoutProps) {
  const [open, setOpen] = useState(false);
  const values = valuesFor(bytes, unknownMask, selection, cursor);
  const summary = values.filter((v) => SUMMARY.includes(v.label));
  const shown = summary.length > 0 ? summary : values.filter((v) => SUMMARY_SMALL.includes(v.label));
  const focus = cursor ?? selection?.offset ?? null;

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && setOpen(false);
    const onDown = (e: MouseEvent) => {
      if (!(e.target as HTMLElement).closest('[data-readout]')) setOpen(false);
    };
    document.addEventListener('keydown', onKey);
    document.addEventListener('mousedown', onDown);
    return () => {
      document.removeEventListener('keydown', onKey);
      document.removeEventListener('mousedown', onDown);
    };
  }, [open]);

  return (
    <>
      <button
        type="button"
        data-readout
        aria-expanded={open}
        title="Все значения выделения"
        onClick={() => setOpen(!open)}
        className="flex items-center gap-4 whitespace-nowrap hover:text-fg"
      >
        <Item label="курсор" value={focus === null ? '—' : formatOffset(focus)} />
        <Item label="выдел." value={selection ? `${selection.length} Б` : '—'} />
        {shown.map((v) => (
          <Item key={v.label} label={v.label} value={v.value} />
        ))}
      </button>
      {open && createPortal(<Converter bytes={bytes} unknownMask={unknownMask} segments={segments} selection={selection} values={values} cursor={cursor} />, document.body)}
    </>
  );
}

function Item({ label, value }: { label: string; value: string }) {
  return (
    <span>
      <span className="mr-1.5 text-muted">{label}</span>
      {value}
    </span>
  );
}

interface ConverterProps extends Omit<ByteReadoutProps, 'cursor'> {
  cursor: number | null;
  values: DecodedValue[];
}

const ROWS: { label: string; le: string; be?: string }[] = [
  { label: 'u8', le: 'u8' },
  { label: 'i8', le: 'i8' },
  { label: 'u16', le: 'u16le', be: 'u16be' },
  { label: 'i16', le: 'i16le', be: 'i16be' },
  { label: 'u32', le: 'u32le', be: 'u32be' },
  { label: 'i32', le: 'i32le', be: 'i32be' },
  { label: 'f32', le: 'f32le', be: 'f32be' },
];

/** Converter: every interpretation of the bytes from the start of the selection, little and big endian. */
function Converter({ bytes, unknownMask, segments, selection, cursor, values }: ConverterProps) {
  const range: ByteRange | null = selection ?? (cursor === null ? null : { offset: cursor, length: 1 });
  const slice = range ? bytes.subarray(range.offset, range.offset + range.length) : bytes.subarray(0, 0);
  const sliceMask = range ? unknownMask.subarray(range.offset, range.offset + range.length) : unknownMask.subarray(0, 0);
  const packets = range ? packetsOf(segments, range) : [];
  const value = (label?: string) => values.find((v) => v.label === label)?.value;

  return (
    <div
      data-readout
      role="dialog"
      aria-label="Значения выделения"
      className="fixed right-4 bottom-8 z-40 w-[420px] rounded-ctl border border-line-strong bg-surface p-4 shadow-pop"
    >
      {range === null ? (
        <p className="text-muted">Наведите курсор на байт или выделите диапазон.</p>
      ) : (
        <div className="flex flex-col gap-3">
          <div>
            <div className="mb-1 flex items-baseline justify-between gap-3">
              <p className="font-mono text-xs">
                {formatRange(range)}
                {packets.length > 0 && <span className="text-muted"> пакеты #{packets.join(', #')}</span>}
              </p>
              <CopyButton value={toHexMasked(slice, sliceMask)} label="hex" />
            </div>
            <p className="font-mono text-xs break-all">
              {toHexMasked(slice.subarray(0, PREVIEW_LIMIT), sliceMask.subarray(0, PREVIEW_LIMIT))}
              {slice.length > PREVIEW_LIMIT && <span className="text-muted"> и ещё {slice.length - PREVIEW_LIMIT} Б</span>}
            </p>
            <p className="font-mono text-xs break-all text-muted">{toAscii(slice.subarray(0, PREVIEW_LIMIT), '.', sliceMask)}</p>
          </div>
          <table className="w-full border-collapse font-mono text-xs">
            <thead>
              <tr className="text-left text-xs text-muted">
                <th className="h-6 border-b border-line font-normal" />
                <th className="h-6 border-b border-line text-right font-normal">little-endian</th>
                <th className="h-6 border-b border-line text-right font-normal">big-endian</th>
              </tr>
            </thead>
            <tbody>
              {ROWS.map((row) => (
                <tr key={row.label} className="border-b border-line last:border-b-0">
                  <td className="h-6 text-muted">{row.label}</td>
                  <Cell value={value(row.le)} />
                  {row.be ? <Cell value={value(row.be)} /> : <td />}
                </tr>
              ))}
            </tbody>
          </table>
          <p className="text-xs text-muted">
            {selection ? 'Только значения, целиком попадающие в выделение.' : 'Значения с байта под курсором.'} Неизвестные байты не
            превращаются в числа.
          </p>
        </div>
      )}
    </div>
  );
}

function Cell({ value }: { value?: string }) {
  return <td className={cn('text-right', value === undefined && 'text-muted', value === '??' && 'text-unknown')}>{value ?? '—'}</td>;
}
