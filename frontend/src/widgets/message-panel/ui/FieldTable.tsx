import { CaretDownIcon, CaretRightIcon } from '@phosphor-icons/react';
import { useState } from 'react';
import { MeaningMark, type Message, type ParsedField } from '@/entities/message';
import { cn } from '@/shared/lib';
import { fieldColor } from '../model/palette';

export interface FieldTableProps {
  message: Message;
  /** Called with a message-relative range. */
  onSelectRange: (offset: number, length: number) => void;
}

/** Parsed fields with their status: what is derived by rules vs. what remains a hypothesis. */
export function FieldTable({ message, onSelectRange }: FieldTableProps) {
  return (
    <div className="flex flex-col gap-2">
      {message.issues.length > 0 && (
        <ul className="px-3 py-2 text-xs text-warn">
          {message.issues.map((i) => (
            <li key={i}>{i}</li>
          ))}
        </ul>
      )}
      <table className="w-full text-[12px]">
        <thead>
          <tr className="text-left text-xs text-muted">
            <th className="py-1 pr-2 pl-1 font-normal">Поле</th>
            <th className="py-1 pr-2 font-normal">@ · тип</th>
            <th className="py-1 pr-2 font-normal">Значение</th>
            <th className="py-1 font-normal">Смысл</th>
          </tr>
        </thead>
        <tbody>
          {message.fields.map((f, i) => (
            <FieldRow key={f.path} field={f} color={fieldColor(i)} depth={0} onSelectRange={onSelectRange} />
          ))}
          {message.unknown.map((u) => (
            <tr
              key={`u${u.offset}`}
              onClick={() => onSelectRange(u.offset, u.length)}
              className="cursor-pointer border-t border-line/60 text-muted hover:bg-surface-2"
            >
              <td className="py-1 pr-2 pl-1 italic">неизвестно</td>
              <td className="pr-2 font-mono whitespace-nowrap">
                {u.offset} · {u.length} Б
              </td>
              <td colSpan={2} className="text-subtle">
                не описано полями
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function FieldRow({
  field,
  color,
  depth,
  onSelectRange,
}: {
  field: ParsedField;
  color?: string;
  depth: number;
  onSelectRange: (offset: number, length: number) => void;
}) {
  const [open, setOpen] = useState(depth === 0 && (field.children?.length ?? 0) <= 4);
  const hasChildren = (field.children?.length ?? 0) > 0;
  return (
    <>
      <tr
        onClick={() => onSelectRange(field.offset, field.length)}
        className={cn('cursor-pointer border-t border-line/60 align-top hover:bg-surface-2', field.status === 'violation' && 'bg-danger/8')}
      >
        <td className="py-1 pr-2 whitespace-nowrap" style={{ paddingLeft: 4 + depth * 14 }}>
          <span className="inline-flex items-center gap-1.5">
            {hasChildren ? (
              <button
                type="button"
                onClick={(e) => {
                  e.stopPropagation();
                  setOpen(!open);
                }}
                className="text-subtle hover:text-fg"
              >
                {open ? <CaretDownIcon size={12} /> : <CaretRightIcon size={12} />}
              </button>
            ) : (
              color && <span className="size-2 " style={{ background: color }} />
            )}
            <span className="font-mono">{field.name}</span>
          </span>
        </td>
        <td className="py-1 pr-2 font-mono whitespace-nowrap text-subtle">
          {field.offset} · {field.type}
        </td>
        <td className="py-1 pr-2">
          <span className={cn('font-mono', field.status === 'violation' ? 'text-danger' : field.status === 'unknown' ? 'text-warn' : 'text-fg')}>
            {field.display}
          </span>
          {field.issue && <div className="text-[11px] text-danger">{field.issue}</div>}
        </td>
        <td className="py-1">
          {field.meaning && <div className="text-muted">{field.meaning}</div>}
          {field.meaningStatus && field.type !== 'struct' && <MeaningMark status={field.meaningStatus} />}
        </td>
      </tr>
      {open &&
        field.children?.map((c) => <FieldRow key={c.path} field={c} depth={depth + 1} onSelectRange={onSelectRange} />)}
    </>
  );
}
