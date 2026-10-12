import { CaretDownIcon, CaretRightIcon } from '@phosphor-icons/react';
import { useState } from 'react';
import { fieldRange, type FieldLink, type Message, type ParsedField } from '@/entities/message';
import { cn, type ByteRange } from '@/shared/lib';
import { fieldSwatchClass, Status } from '@/shared/ui';

export interface FieldTableProps {
  message: Message;
  /** Selection in the directed stream: the row of the selected field is marked. */
  selection: ByteRange | null;
  link: FieldLink | null;
  /** Receives a stream range. */
  onSelectRange: (range: ByteRange) => void;
  onLink: (link: FieldLink | null) => void;
}

const sameRange = (a: ByteRange | null, b: ByteRange) => a !== null && a.offset === b.offset && a.length === b.length;

/**
 * Parsed fields: derived by rules vs. left as a hypothesis. Hovering a row lights up its bytes, a click selects them.
 * A field is one line (name, offset, type, value); what it means goes on a quiet second line, so the
 * columns never squeeze the text.
 */
export function FieldTable({ message, selection, link, onSelectRange, onLink }: FieldTableProps) {
  return (
    <div className="flex flex-col gap-2">
      {message.issues.length > 0 && (
        <ul className="flex flex-col gap-1 text-xs">
          {message.issues.map((issue) => (
            <li key={issue}>
              <Status tone="warn">{issue}</Status>
            </li>
          ))}
        </ul>
      )}
      <table className="w-full border-collapse text-xs">
        <thead>
          <tr className="legend text-left text-muted">
            <th className="h-6 border-b border-line pr-3 font-normal">Поле</th>
            <th className="h-6 border-b border-line pr-2 text-right font-normal" title="Смещение в сообщении">
              @
            </th>
            <th className="h-6 border-b border-line px-2 font-normal">Тип</th>
            <th className="h-6 border-b border-line pr-2 font-normal">Значение</th>
          </tr>
        </thead>
        <tbody>
          {message.fields.map((f, i) => (
            <FieldRow key={f.path} message={message} field={f} tone={i} depth={0} selection={selection} link={link} onSelectRange={onSelectRange} onLink={onLink} />
          ))}
          {message.unknown.map((u) => {
            const range = { offset: message.offset + u.offset, length: u.length };
            return (
              <tr
                key={`u${u.offset}`}
                onClick={() => onSelectRange(range)}
                className={cn('cursor-pointer border-b border-line text-muted hover:bg-surface-2', sameRange(selection, range) && 'bg-selection hover:bg-selection')}
              >
                <td className="py-1.5 pr-2 pl-4">не описано</td>
                <td className="pr-2 text-right font-mono">{u.offset}</td>
                <td className="px-2 font-mono">{u.length} Б</td>
                <td />
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

interface FieldRowProps extends Omit<FieldTableProps, 'selection'> {
  selection: ByteRange | null;
  field: ParsedField;
  tone: number;
  depth: number;
}

function FieldRow({ message, field, tone, depth, selection, link, onSelectRange, onLink }: FieldRowProps) {
  const [open, setOpen] = useState(depth === 0 && (field.children?.length ?? 0) <= 4);
  const children = field.children ?? [];
  const range = fieldRange(message, field);
  const selected = sameRange(selection, range);
  const linked = link?.messageIndex === message.index && link.path === field.path;
  const violation = field.status === 'violation';
  const status = field.type === 'struct' ? undefined : field.meaningStatus;
  const hasMeaning = !!field.meaning || status === 'hypothesis';
  // The second line (meaning, issue) belongs to the same field: it shares the highlight and the bottom rule.
  const tail = hasMeaning || !!field.issue;
  const rowClass = cn(
    'cursor-pointer align-top',
    selected ? 'bg-selection' : linked && 'bg-surface-2',
    violation && 'text-danger',
  );
  const indent = 16 + depth * 12;
  const hover = {
    onClick: () => onSelectRange(range),
    onMouseEnter: () => onLink({ messageIndex: message.index, path: field.path, range }),
    onMouseLeave: () => onLink(null),
  };

  return (
    <>
      <tr {...hover} className={cn(rowClass, !tail && 'border-b border-line')}>
        <td className="pt-1.5 pr-3 whitespace-nowrap" style={{ paddingLeft: indent }}>
          <span className="relative inline-flex items-center gap-2">
            {children.length > 0 ? (
              <button
                type="button"
                aria-label={open ? 'Свернуть' : 'Развернуть'}
                onClick={(e) => {
                  e.stopPropagation();
                  setOpen(!open);
                }}
                className="absolute -left-4 text-muted hover:text-fg"
              >
                {open ? <CaretDownIcon size={12} /> : <CaretRightIcon size={12} />}
              </button>
            ) : null}
            {depth === 0 && <i aria-hidden className={cn('size-2 shrink-0', violation ? 'bg-danger' : fieldSwatchClass(tone))} />}
            <span className={cn('font-mono', depth > 0 && 'text-muted')}>{field.name}</span>
          </span>
        </td>
        <td className="pt-1.5 pr-2 text-right font-mono text-muted">{field.offset}</td>
        <td className="px-2 pt-1.5 font-mono whitespace-nowrap text-muted">{field.type}</td>
        <td className="pt-1.5 pr-2">
          <span className={cn('font-mono whitespace-nowrap', field.status === 'unknown' && 'text-warn')}>{field.display}</span>
        </td>
      </tr>
      {tail && (
        <tr {...hover} className={cn(rowClass, 'border-b border-line')}>
          <td colSpan={4} className="pr-2 pb-1.5 text-2xs leading-4" style={{ paddingLeft: indent + (depth === 0 ? 16 : 0) }}>
            {field.issue && <p className="text-danger">{field.issue}</p>}
            {hasMeaning && <Meaning field={field} status={status} violation={violation} />}
          </td>
        </tr>
      )}
      {open &&
        children.map((c) => (
          <FieldRow key={c.path} message={message} field={c} tone={tone} depth={depth + 1} selection={selection} link={link} onSelectRange={onSelectRange} onLink={onLink} />
        ))}
    </>
  );
}

/** Confirmed meaning is a quiet green square; a hypothesis also says so in words; structure alone is silent. */
function Meaning({ field, status, violation }: { field: ParsedField; status: ParsedField['meaningStatus']; violation: boolean }) {
  return (
    <span className="inline-flex items-start gap-1.5">
      {status === 'confirmed' && <i aria-hidden title="Назначение подтверждено" className="mt-[5px] size-[7px] shrink-0 bg-ok" />}
      {status === 'hypothesis' && <i aria-hidden title="Назначение: непроверенная гипотеза" className="mt-[5px] size-[7px] shrink-0 bg-warn" />}
      <span className={violation ? '' : 'text-muted'}>
        {field.meaning}
        {status === 'hypothesis' && <span className="ml-1.5 text-fg">гипотеза</span>}
      </span>
    </span>
  );
}
