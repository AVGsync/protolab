import { ArrowLeftIcon } from '@phosphor-icons/react';
import { useState, type Ref } from 'react';
import { Link } from 'react-router';
import { DiagnosticList } from '@/entities/capture';
import type { Interpretation } from '@/entities/interpretation';
import {
  DirectionSwitch,
  EndpointPair,
  SessionAnomalySentence,
  SessionStatusMark,
  describeAnomalies,
  sessionAnomalies,
  type TcpSession,
} from '@/entities/session';
import { CreateObservationButton } from '@/features/create-observation';
import type { SelectionDetails } from '@/widgets/byte-readout';
import type { Direction } from '@/shared/api';
import { routes } from '@/shared/config';
import { useClickOutside } from '@/shared/lib';
import { Select, Status } from '@/shared/ui';

export const NO_VERSION = 'none';

export interface ContextBarProps {
  projectId: string;
  session: TcpSession;
  direction: Direction;
  onDirection: (direction: Direction) => void;
  interpretations: Interpretation[];
  versionId: string;
  /** Number of the version when it is not the latest of its interpretation. */
  staleVersion?: number;
  onVersion: (versionId: string) => void;
  versionRef: Ref<HTMLSelectElement>;
  /** The selected bytes with their provenance; the observation action is enabled only when set. */
  selection: SelectionDetails | null;
}

/** Back link, session identity, one sentence about anomalies, direction, version and the observation action. */
export function ContextBar({
  projectId,
  session: s,
  direction,
  onDirection,
  interpretations,
  versionId,
  staleVersion,
  onVersion,
  versionRef,
  selection,
}: ContextBarProps) {
  return (
    <div className="flex h-11 shrink-0 items-center gap-4 border-b border-line bg-surface px-4">
      <Link to={routes.sessions(projectId)} className="text-muted hover:text-fg" title="К списку сессий" aria-label="К списку сессий">
        <ArrowLeftIcon size={16} />
      </Link>
      <span className="shrink-0 font-mono text-[13px] font-semibold whitespace-nowrap">{s.id}</span>
      <EndpointPair client={s.client} server={s.server} className="shrink-0" />
      <Anomalies session={s} />

      <div className="ml-auto flex shrink-0 items-center gap-2">
        <DirectionSwitch value={direction} onChange={onDirection} bytes={s.bytes} />
        <Select
          ref={versionRef}
          value={versionId}
          onChange={(e) => onVersion(e.target.value)}
          className="w-44"
          title="Интерпретация для кадрирования и разбора (V)"
          aria-label="Интерпретация"
        >
          <option value={NO_VERSION}>Сырой поток</option>
          {interpretations.map((i) => (
            <option key={i.id} value={i.latestVersionId}>
              {i.name} v{i.latestVersion}
            </option>
          ))}
          {staleVersion !== undefined && <option value={versionId}>v{staleVersion} (не последняя)</option>}
        </Select>
        <CreateObservationButton projectId={projectId} source={selection?.source ?? null} previewHex={selection?.hex ?? ''} hasUnknownBytes={selection?.hasUnknown} />
      </div>
    </div>
  );
}

/** The sentence opens the details: what each anomaly means for the analysis, and the capture diagnostics. */
function Anomalies({ session: s }: { session: TcpSession }) {
  const [open, setOpen] = useState(false);
  const ref = useClickOutside<HTMLDivElement>(() => setOpen(false), open);
  const anomalies = sessionAnomalies(s.flags, s.roles);
  if (anomalies.length === 0 && s.diagnostics.length === 0) return <SessionStatusMark status={s.status} />;

  return (
    <div ref={ref} className="relative min-w-0">
      <button
        type="button"
        onClick={() => setOpen(!open)}
        aria-expanded={open}
        title={describeAnomalies(anomalies) || 'Диагностика записи'}
        className="flex max-w-full min-w-0 items-center text-muted hover:text-fg"
      >
        {anomalies.length > 0 ? <SessionAnomalySentence flags={s.flags} roles={s.roles} className="text-inherit" /> : <span>диагностика</span>}
      </button>
      {open && (
        <div className="absolute top-8 left-0 z-40 w-[480px] rounded-ctl border border-line-strong bg-surface p-4 shadow-pop">
          {anomalies.length > 0 && (
            <ul className="flex flex-col gap-1.5">
              {anomalies.map((a) => (
                <li key={a.text} className="flex gap-3">
                  <Status tone={a.tone} className="shrink-0">
                    {a.text}
                  </Status>
                  <span className="text-muted">{a.hint}</span>
                </li>
              ))}
            </ul>
          )}
          {s.diagnostics.length > 0 && <DiagnosticList items={s.diagnostics} className={anomalies.length > 0 ? 'mt-3 border-t border-line pt-3' : undefined} />}
        </div>
      )}
    </div>
  );
}
