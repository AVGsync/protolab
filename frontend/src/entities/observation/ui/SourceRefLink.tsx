import { Link } from 'react-router';
import type { SourceRef } from '@/shared/api';
import { routes } from '@/shared/config';
import { formatRange } from '@/shared/lib';

/** Link back to the exact source bytes of a derived result. */
export function SourceRefLink({ projectId, source, versionId }: { projectId: string; source: SourceRef; versionId?: string }) {
  return (
    <Link
      to={routes.session(projectId, source.sessionId, {
        dir: source.direction,
        sel: { offset: source.offset, length: source.length },
        v: versionId,
      })}
      className="link font-mono text-xs whitespace-nowrap"
      title={`Захват ${source.captureId} (sha256 ${source.captureSha256.slice(0, 12)}…), пакеты #${source.packetNumbers.join(', #')}`}
    >
      {source.sessionId}/{source.direction} {formatRange(source)}
    </Link>
  );
}
