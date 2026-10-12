import type { ReactNode } from 'react';
import { PROJECT_SECTIONS } from '@/shared/config';
import { Keycap, Modal } from '@/shared/ui';
import type { ContextKey } from '../model/keys';

interface Row {
  keys: ReactNode;
  text: string;
}

function Group({ title, rows }: { title: string; rows: Row[] }) {
  return (
    <section>
      <h3 className="mb-1.5 text-xs text-muted">{title}</h3>
      <ul className="flex flex-col">
        {rows.map((row, i) => (
          <li key={i} className="flex items-center gap-3 border-b border-line py-1.5 last:border-b-0">
            <span className="flex w-24 shrink-0 items-center gap-1">{row.keys}</span>
            <span>{row.text}</span>
          </li>
        ))}
      </ul>
    </section>
  );
}

export interface HelpOverlayProps {
  open: boolean;
  onClose: () => void;
  inProject: boolean;
  contextKeys: ContextKey[];
}

export function HelpOverlay({ open, onClose, inProject, contextKeys }: HelpOverlayProps) {
  return (
    <Modal open={open} onClose={onClose} title="Клавиши" size="md">
      <div className="flex flex-col gap-5">
        {inProject && (
          <Group
            title="Разделы проекта"
            rows={PROJECT_SECTIONS.map((s) => ({ keys: <Keycap>{s.key}</Keycap>, text: s.label }))}
          />
        )}
        {contextKeys.length > 0 && (
          <Group
            title="На этом экране"
            rows={contextKeys.map((k) => ({ keys: <Keycap>{k.key}</Keycap>, text: k.description ?? k.label }))}
          />
        )}
        <Group
          title="Везде"
          rows={[
            { keys: <Keycap>?</Keycap>, text: 'Эти подсказки' },
            { keys: <Keycap>Esc</Keycap>, text: 'Закрыть окно' },
          ]}
        />
        <p className="text-xs text-muted">Клавиши не срабатывают при вводе текста. Буквы работают в любой раскладке.</p>
      </div>
    </Modal>
  );
}
