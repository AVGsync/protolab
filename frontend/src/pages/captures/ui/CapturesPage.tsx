import { CaretDownIcon, CaretRightIcon } from '@phosphor-icons/react';
import { useQuery } from '@tanstack/react-query';
import { Fragment, useState } from 'react';
import { Link } from 'react-router';
import { CaptureStatusMark, captureQueries, DiagnosticList, IntegrityMark, type Capture } from '@/entities/capture';
import { CaptureDropzone, UploadCaptureButton } from '@/features/upload-capture';
import { routes } from '@/shared/config';
import { countLabel, formatBytes, formatDateTime, formatNumber, formatTimestamp, shortHash, useRequiredParams } from '@/shared/lib';
import { CopyButton, ErrorState, Loader, Page, PageHeader, Pane, Status, Table, TBody, Td, Th, THead, Tr } from '@/shared/ui';

/** At most one marker per row: the most important thing about the capture, nothing when all is well. */
function CaptureAttention({ capture }: { capture: Capture }) {
  if (capture.status !== 'ready') return <CaptureStatusMark status={capture.status} />;
  if (capture.integrity === 'mismatch') return <IntegrityMark integrity={capture.integrity} />;
  const warnings = capture.diagnostics.filter((d) => d.level !== 'info').length;
  if (warnings === 0) return null;
  return <Status tone="warn">{countLabel(warnings, ['предупреждение', 'предупреждения', 'предупреждений'])}</Status>;
}

export function CapturesPage() {
  const { projectId } = useRequiredParams('projectId');
  const { data, isLoading, error, refetch } = useQuery(captureQueries.list(projectId));
  const [expanded, setExpanded] = useState<string | null>(null);

  return (
    <Page>
      <PageHeader title="Записи трафика" actions={data && data.items.length > 0 && <UploadCaptureButton projectId={projectId} />} />
      {isLoading && <Loader />}
      {error && <ErrorState error={error} onRetry={refetch} />}
      {data && data.items.length === 0 && <CaptureDropzone projectId={projectId} />}
      {data && data.items.length > 0 && (
        <Pane>
          <Table>
            <THead>
              <tr>
                <Th className="w-8" />
                <Th>Файл</Th>
                <Th>SHA-256</Th>
                <Th className="text-right">Размер</Th>
                <Th className="text-right">Пакеты</Th>
                <Th className="text-right">Сессии</Th>
                <Th>Интервал (UTC)</Th>
                <Th />
              </tr>
            </THead>
            <TBody>
              {data.items.map((c) => {
                const open = expanded === c.id;
                return (
                  <Fragment key={c.id}>
                    <Tr onClick={() => setExpanded(open ? null : c.id)}>
                      <Td className="text-subtle">{open ? <CaretDownIcon size={12} /> : <CaretRightIcon size={12} />}</Td>
                      <Td>
                        <p>{c.fileName}</p>
                        <p className="font-mono text-[11px] text-subtle">
                          {c.format.toUpperCase()} {c.linkType} snaplen {c.snapLen}, импорт {formatDateTime(c.importedAt)}
                        </p>
                      </Td>
                      <Td onClick={(e) => e.stopPropagation()}>
                        <span className="inline-flex items-center gap-1 font-mono text-xs text-muted" title={c.sha256}>
                          {shortHash(c.sha256, 16)}…
                          <CopyButton value={c.sha256} />
                        </span>
                      </Td>
                      <Td className="text-right font-mono text-xs whitespace-nowrap">{formatBytes(c.sizeBytes)}</Td>
                      <Td className="text-right font-mono text-xs">{formatNumber(c.packetCount)}</Td>
                      <Td className="text-right font-mono text-xs whitespace-nowrap" onClick={(e) => e.stopPropagation()}>
                        <Link to={routes.sessions(projectId, { captureId: c.id })} className="link">
                          {c.sessionCount}
                        </Link>
                        <span className="text-subtle"> / {c.targetSessionCount} цел.</span>
                      </Td>
                      <Td className="font-mono text-xs whitespace-nowrap text-muted">
                        {c.firstPacketAt ? `${c.firstPacketAt.slice(0, 10)} ${formatTimestamp(c.firstPacketAt, 0)}–${formatTimestamp(c.lastPacketAt!, 0)}` : '—'}
                      </Td>
                      <Td>
                        <CaptureAttention capture={c} />
                      </Td>
                    </Tr>
                    {open && (
                      <tr className="border-b border-line bg-bg">
                        <td />
                        <td colSpan={7} className="px-3 py-3">
                          <DiagnosticList items={c.diagnostics} />
                        </td>
                      </tr>
                    )}
                  </Fragment>
                );
              })}
            </TBody>
          </Table>
        </Pane>
      )}
    </Page>
  );
}
