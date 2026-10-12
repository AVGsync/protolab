import { Link } from 'react-router';
import { HypothesisStatusBadge, type Hypothesis } from '@/entities/hypothesis';
import type { FieldRule, InterpretationSpec } from '@/entities/interpretation';
import { MeaningBadge } from '@/entities/message';
import { DirectionBadge } from '@/entities/session';
import { routes } from '@/shared/config';

export interface SpecOutlineProps {
  projectId: string;
  spec: InterpretationSpec;
  hypotheses?: Hypothesis[];
}

/** Human-readable view of a spec: what is known, what is a hypothesis and what backs it. */
export function SpecOutline({ projectId, spec, hypotheses = [] }: SpecOutlineProps) {
  const byId = new Map(hypotheses.map((h) => [h.id, h]));
  const f = spec.framing;
  return (
    <div className="flex flex-col gap-4">
      <div className="rounded-md border border-line bg-surface px-3 py-2.5 text-[12.5px]">
        <span className="text-muted">Кадрирование: </span>
        {f.kind === 'length_prefixed' && (
          <span className="font-mono">
            len=u{f.lengthField.size * 8}
            {f.lengthField.endian}@{f.lengthField.offset} · total=len+{f.lengthAdjust}
            {f.sync && ` · sync ${f.sync}`}
          </span>
        )}
        {f.kind === 'delimiter' && <span className="font-mono">разделитель {f.delimiter}</span>}
        {f.kind === 'fixed' && <span className="font-mono">фиксированный размер {f.size} Б</span>}
      </div>

      {spec.messages.map((rule) => (
        <div key={rule.id} className="rounded-md border border-line bg-surface">
          <div className="flex flex-wrap items-center gap-2 border-b border-line px-3 py-2">
            <span className="font-mono text-[12.5px] font-semibold">{rule.id}</span>
            {rule.title && <span className="text-muted">{rule.title}</span>}
            {rule.direction && rule.direction !== 'any' && <DirectionBadge direction={rule.direction} compact />}
            <span className="ml-auto font-mono text-[11px] text-subtle">
              {rule.match.map((m) => `@${m.offset}=${m.hex}`).join(' ∧ ')}
            </span>
          </div>
          <ul className="divide-y divide-line/60">
            {rule.fields.map((field) => (
              <FieldLine key={field.name} field={field} projectId={projectId} byId={byId} />
            ))}
          </ul>
        </div>
      ))}

      {spec.assertions && spec.assertions.length > 0 && (
        <div className="rounded-md border border-line bg-surface px-3 py-2.5">
          <p className="mb-1.5 text-[11px] font-semibold tracking-wider text-subtle uppercase">Проверки по журналу действий</p>
          <ul className="flex flex-col gap-1 text-[12.5px]">
            {spec.assertions.map((a) => (
              <li key={a.id} className="font-mono text-[12px]">
                <span className="text-muted">{a.id}:</span> {a.rule} при «{a.actionKind}» →{' '}
                {a.checks.map((c) => `${c.field} = ${c.equals}`).join(', ')}
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}

function FieldLine({ field, projectId, byId, depth = 0 }: { field: FieldRule; projectId: string; byId: Map<string, Hypothesis>; depth?: number }) {
  const h = field.hypothesis ? byId.get(field.hypothesis) : undefined;
  return (
    <>
      <li className="grid grid-cols-[minmax(110px,160px)_44px_84px_1fr] items-start gap-2 px-3 py-1.5 text-[12.5px]">
        <span className="font-mono" style={{ paddingLeft: depth * 14 }}>
          {field.name}
        </span>
        <span className="font-mono text-subtle">@{field.offset}</span>
        <span className="font-mono text-subtle">
          {field.type}
          {field.endian && field.type !== 'u8' && field.type !== 'i8' ? field.endian : ''}
          {field.scale ? ` ×${field.scale}` : ''}
        </span>
        <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
          <span className="text-muted">
            {field.meaning ?? ''}
            {field.checksum && <span className="ml-1 font-mono text-[11px] text-info">[{field.checksum}]</span>}
          </span>
          {field.status && <MeaningBadge status={field.status} />}
          {h && (
            <Link to={routes.hypotheses(projectId, h.id)} title={`${h.id}: ${h.title}`} className="inline-flex items-center gap-1">
              <span className="font-mono text-[10.5px] text-subtle">{h.id}</span>
              <HypothesisStatusBadge status={h.status} />
            </Link>
          )}
        </span>
      </li>
      {field.items?.map((item) => <FieldLine key={item.name} field={item} projectId={projectId} byId={byId} depth={depth + 1} />)}
    </>
  );
}
