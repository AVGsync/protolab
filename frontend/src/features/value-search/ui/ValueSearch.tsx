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
import { Button, Disclosure, Input, Segmented, Select } from '@/shared/ui';

export interface ValueSearchProps {
  bytes: Uint8Array;
  /** 1 = unknown byte (gap); hits overlapping unknown bytes are discarded. */
  unknownMask: Uint8Array;
  onHits: (hits: ByteRange[]) => void;
  onSelect: (range: ByteRange) => void;
}

const DEFAULT_ENCODINGS: IntEncoding[] = ['u8', 'u16le', 'u16be', 'i16le', 'i16be', 'u32le'];

/**
 * "After «set 21» the value 21 appears in the exchange": search a known value
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

  const toggle = (enc: IntEncoding) => setEncodings((list) => (list.includes(enc) ? list.filter((e) => e !== enc) : [...list, enc]));

  return (
    <div className="flex min-h-0 flex-col">
      <form onSubmit={search} className="flex flex-col gap-2.5 px-4 py-3">
        <Segmented
          aria-label="Что искать"
          value={mode}
          onChange={setMode}
          items={[
            { id: 'number', label: 'Число' },
            { id: 'hex', label: 'Байты (hex)' },
          ]}
          className="self-start"
        />
        <div className="flex gap-2">
          <Input
            autoFocus
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={mode === 'number' ? '21 или 22.4' : 'A5 02'}
            aria-label="Искомое значение"
            className="min-w-0 flex-1 font-mono"
          />
          {mode === 'number' && (
            <Select value={scale} onChange={(e) => setScale(Number(e.target.value))} title="Масштаб: значение × k" className="w-16">
              {[1, 10, 100].map((k) => (
                <option key={k} value={k}>
                  ×{k}
                </option>
              ))}
            </Select>
          )}
          <Button type="submit" variant="primary">
            Найти
          </Button>
        </div>
        {mode === 'number' && (
          <Disclosure summary={`Кодировки, ${encodings.length} из ${INT_ENCODINGS.length}`}>
            <div className="flex flex-wrap gap-1" role="group" aria-label="Кодировки">
              {INT_ENCODINGS.map((enc) => (
                <button
                  key={enc}
                  type="button"
                  aria-pressed={encodings.includes(enc)}
                  onClick={() => toggle(enc)}
                  className={cn(
                    'h-5 rounded-ctl border px-1.5 font-mono text-2xs',
                    encodings.includes(enc) ? 'border-line-strong bg-surface-2 text-fg' : 'border-line text-muted hover:text-fg',
                  )}
                >
                  {enc}
                </button>
              ))}
            </div>
          </Disclosure>
        )}
        {error && <p className="text-xs text-danger">{error}</p>}
      </form>

      {hits && (
        <div className="flex min-h-0 flex-col border-t border-line">
          <p className="px-4 py-1.5 text-xs text-muted">
            Найдено: {hits.length}.{hits.length > 0 && ' Совпадение значения — повод для гипотезы, а не доказательство.'}
          </p>
          <ul className="min-h-0 overflow-y-auto">
            {hits.map((h) => (
              <li key={`${h.offset}:${h.length}`}>
                <button
                  type="button"
                  onClick={() => onSelect(h)}
                  className="flex h-7 w-full items-center gap-3 border-t border-line px-4 text-left font-mono text-xs hover:bg-surface-2"
                >
                  <span className="w-14 text-muted">{formatOffset(h.offset)}</span>
                  <span>{toHex(bytes.subarray(h.offset, h.offset + h.length))}</span>
                  <span className="ml-auto text-2xs text-muted">{h.encodings.join(' ')}</span>
                </button>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}
