import { Link } from 'react-router';
import { HYPOTHESIS_STATUS, type Hypothesis } from '@/entities/hypothesis';
import { effectiveMeaningStatus, type FieldRule, type InterpretationSpec } from '@/entities/interpretation';
import { MeaningMark } from '@/entities/message';
import { DirectionLabel } from '@/entities/session';
import { routes } from '@/shared/config';
import { cn } from '@/shared/lib';
import { Arrow } from '@/shared/ui';

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
      <div className="border border-line bg-surface px-3 py-2.5 text-xs">
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
        <div key={rule.id} className="border border-line bg-surface">
          <div className="flex flex-wrap items-center gap-2 border-b border-line px-3 py-2">
            <span className="font-mono text-xs font-semibold">{rule.id}</span>
            {rule.title && <span className="text-muted">{rule.title}</span>}
            {rule.direction && rule.direction !== 'any' && <DirectionLabel direction={rule.direction} compact />}
            <span className="ml-auto font-mono text-2xs text-muted">
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
        <div className="border border-line bg-surface px-3 py-2.5">
          <p className="mb-1.5 text-xs text-muted">Проверки по журналу действий</p>
          <ul className="flex flex-col gap-1 text-xs">
            {spec.assertions.map((a) => (
              <li key={a.id} className="font-mono text-xs">
                <span className="text-muted">{a.id}:</span> {a.rule} при «{a.actionKind}» <Arrow />{' '}
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
      <li className="grid grid-cols-[minmax(110px,160px)_44px_84px_1fr] items-start gap-2 px-3 py-1.5 text-xs">
        <span className="font-mono" style={{ paddingLeft: depth * 14 }}>
          {field.name}
        </span>
        <span className="font-mono text-muted">@{field.offset}</span>
        <span className="font-mono text-muted">
          {field.type}
          {field.endian && field.type !== 'u8' && field.type !== 'i8' ? field.endian : ''}
          {field.scale ? ` ×${field.scale}` : ''}
        </span>
        <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
          <span className="text-muted">
            {field.meaning ?? ''}
            {field.checksum && <span className="ml-1 font-mono text-2xs text-info">[{field.checksum}]</span>}
          </span>
          {h && (
            <Link
              to={routes.hypotheses(projectId, h.id)}
              title={`${h.id}: ${h.title} (${HYPOTHESIS_STATUS[h.status].label})`}
              className={cn('font-mono text-2xs hover:underline', h.status === 'refuted' ? 'text-danger' : 'text-muted')}
            >
              {h.id}
            </Link>
          )}
          {/* Structure alone is the normal state: only a claim about the meaning gets a mark. */}
          {effectiveMeaningStatus(field) !== 'structural' && <MeaningMark status={effectiveMeaningStatus(field)} />}
        </span>
      </li>
      {field.items?.map((item) => <FieldLine key={item.name} field={item} projectId={projectId} byId={byId} depth={depth + 1} />)}
    </>
  );
}
