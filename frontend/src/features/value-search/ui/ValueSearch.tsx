import { useState, type FormEvent } from 'react';
import {
  cn,
  findValue,
  formatOffset,
  INT_ENCODINGS,
  indexOfAll,
  maskHasAny,
  parseHex,
  toHex,
  type ByteRange,
  type IntEncoding,
  type ValueHit,
} from '@/shared/lib';
import { Button, Input } from '@/shared/ui';

export interface ValueSearchProps {
  bytes: Uint8Array;
  /** 1 = unknown byte (gap); hits overlapping unknown bytes are discarded. */
  unknownMask: Uint8Array;
  onHits: (hits: ByteRange[]) => void;
  onSelect: (range: ByteRange) => void;
}

const DEFAULT_ENCODINGS: IntEncoding[] = ['u8', 'u16le', 'u16be', 'i16le', 'i16be', 'u32le'];

/**
 * "After «set 21» the value 21 appears in the exchange" — search a known value
 * in every common encoding (or a raw hex pattern) to start a hypothesis.
 */
export function ValueSearch({ bytes, unknownMask, onHits, onSelect }: ValueSearchProps) {
  const [mode, setMode] = useState<'number' | 'hex'>('number');
  const [query, setQuery] = useState('');
  const [scale, setScale] = useState(1);
  const [encodings, setEncodings] = useState<IntEncoding[]>(DEFAULT_ENCODINGS);
  const [hits, setHits] = useState<ValueHit[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  const search = (e: FormEvent) => {
    e.preventDefault();
    setError(null);
    let result: ValueHit[];
    if (mode === 'number') {
      const n = Number(query.replace(',', '.'));
      if (!query.trim() || !Number.isFinite(n)) return setError('Введите число');
      const raw = Math.round(n * scale);
      if (Math.abs(raw - n * scale) > 1e-6) return setError(`${n} × ${scale} не является целым`);
      result = findValue(bytes, raw, encodings, unknownMask);
    } else {
      const needle = parseHex(query);
      if (!needle) return setError('Некорректный hex, пример: A5 02');
      result = indexOfAll(bytes, needle)
        .filter((offset) => !maskHasAny(unknownMask, { offset, length: needle.length }))
        .map((offset) => ({ offset, length: needle.length, encodings: [] }));
    }
    setHits(result);
    onHits(result);
  };

  const toggle = (enc: IntEncoding) =>
    setEncodings((list) => (list.includes(enc) ? list.filter((e) => e !== enc) : [...list, enc]));

  return (
    <div className="flex flex-col gap-3">
      <form onSubmit={search} className="flex flex-col gap-2.5">
        <div className="flex gap-1 rounded-ctl border border-line-strong bg-bg p-0.5 text-xs">
          {(['number', 'hex'] as const).map((m) => (
            <button
              key={m}
              type="button"
              onClick={() => setMode(m)}
              className={cn('flex-1 px-2 py-1', mode === m ? 'bg-surface-2 text-fg' : 'text-muted hover:text-fg')}
            >
              {m === 'number' ? 'Число' : 'Байты (hex)'}
            </button>
          ))}
        </div>
        <div className="flex gap-2">
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={mode === 'number' ? '21 или 22.4' : 'A5 02'}
            className="min-w-0 flex-1 font-mono"
          />
          {mode === 'number' && (
            <select
              value={scale}
              onChange={(e) => setScale(Number(e.target.value))}
              className="h-7 rounded-ctl border border-line-strong bg-bg px-1.5 text-xs"
              title="Масштаб: значение × k"
            >
              {[1, 10, 100].map((k) => (
                <option key={k} value={k}>
                  ×{k}
                </option>
              ))}
            </select>
          )}
          <Button type="submit" variant="primary">
            Найти
          </Button>
        </div>
        {mode === 'number' && (
          <div className="flex flex-wrap gap-1">
            {INT_ENCODINGS.map((enc) => (
              <button
                key={enc}
                type="button"
                onClick={() => toggle(enc)}
                className={cn(
                  'border px-1.5 py-0.5 font-mono text-[10.5px]',
                  encodings.includes(enc) ? 'border-fg bg-surface-2 text-fg' : 'border-line text-subtle',
                )}
              >
                {enc}
              </button>
            ))}
          </div>
        )}
        {error && <p className="text-xs text-danger">{error}</p>}
      </form>

      {hits && (
        <div>
          <p className="mb-1.5 text-xs text-muted">
            Найдено: {hits.length}
            {hits.length > 0 && ' · совпадение — повод для гипотезы, а не доказательство'}
          </p>
          <ul className="flex max-h-72 flex-col gap-0.5 overflow-y-auto">
            {hits.map((h) => (
              <li key={`${h.offset}:${h.length}`}>
                <button
                  type="button"
                  onClick={() => onSelect(h)}
                  className="flex w-full items-center gap-2 px-2 py-1 text-left hover:bg-surface-2"
                >
                  <span className="font-mono text-[11.5px] text-info">{formatOffset(h.offset)}</span>
                  <span className="font-mono text-[11.5px] text-fg">{toHex(bytes.subarray(h.offset, h.offset + h.length))}</span>
                  <span className="ml-auto font-mono text-[10.5px] text-subtle">{h.encodings.join(' ')}</span>
                </button>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}
