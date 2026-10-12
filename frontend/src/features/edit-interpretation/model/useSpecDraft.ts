import { useMemo, useState } from 'react';
import type { InterpretationSpec } from '@/entities/interpretation';

export interface SpecDraft {
  text: string;
  setText: (text: string) => void;
  reset: (spec: InterpretationSpec) => void;
  /** Parsed spec when the text is valid JSON. */
  spec: InterpretationSpec | null;
  syntaxError: string | null;
  dirty: boolean;
}

export const formatSpec = (spec: InterpretationSpec): string => JSON.stringify(spec, null, 2);

/** Editable JSON text of an interpretation spec with parse state. */
export function useSpecDraft(initial: InterpretationSpec | undefined): SpecDraft {
  const [base, setBase] = useState(() => (initial ? formatSpec(initial) : ''));
  const [text, setText] = useState(base);

  const { spec, syntaxError } = useMemo(() => {
    if (!text.trim()) return { spec: null, syntaxError: 'Пустая спецификация' };
    try {
      return { spec: JSON.parse(text) as InterpretationSpec, syntaxError: null };
    } catch (err) {
      return { spec: null, syntaxError: err instanceof Error ? err.message : 'Некорректный JSON' };
    }
  }, [text]);

  return {
    text,
    setText,
    reset: (next) => {
      const formatted = formatSpec(next);
      setBase(formatted);
      setText(formatted);
    },
    spec,
    syntaxError,
    dirty: text !== base,
  };
}

export const STARTER_SPEC: InterpretationSpec = {
  format: 'protolab/v1',
  protocol: 'NewProtocol',
  framing: {
    kind: 'length_prefixed',
    lengthField: { offset: 3, size: 2, endian: 'le' },
    lengthAdjust: 6,
    sync: 'A5',
  },
  messages: [
    {
      id: 'unknown_frame',
      title: 'Кадр (структура уточняется)',
      direction: 'any',
      match: [{ offset: 0, hex: 'A5' }],
      fields: [
        { name: 'magic', offset: 0, type: 'u8', status: 'hypothesis' },
        { name: 'length', offset: 3, type: 'u16', endian: 'le', status: 'hypothesis' },
      ],
    },
  ],
};
