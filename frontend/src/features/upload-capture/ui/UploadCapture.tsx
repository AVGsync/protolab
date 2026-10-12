import { useState, type DragEvent } from 'react';
import { cn } from '@/shared/lib';
import { FileButton } from '@/shared/ui';
import { useUploadCapture } from '../model/useUploadCapture';

const ACCEPT = '.pcap,.pcapng,.cap';

export function UploadCaptureButton({ projectId }: { projectId: string }) {
  const upload = useUploadCapture(projectId);
  return (
    <FileButton variant="primary" accept={ACCEPT} loading={upload.isPending} onFile={(f) => upload.mutate(f)}>
      Загрузить запись
    </FileButton>
  );
}

/** Drop zone for PCAP/PCAPNG files. The original file is stored unchanged. */
export function CaptureDropzone({ projectId, className }: { projectId: string; className?: string }) {
  const upload = useUploadCapture(projectId);
  const [over, setOver] = useState(false);

  const onDrop = (e: DragEvent) => {
    e.preventDefault();
    setOver(false);
    for (const file of e.dataTransfer.files) upload.mutate(file);
  };

  return (
    <div
      onDragOver={(e) => {
        e.preventDefault();
        setOver(true);
      }}
      onDragLeave={() => setOver(false)}
      onDrop={onDrop}
      className={cn(
        'relative flex flex-col items-center justify-center gap-2 border border-dashed px-6 py-14 text-center transition-colors',
        over ? 'border-fg' : 'border-line-strong',
        className,
      )}
    >
      <div aria-hidden className="bg-graph absolute inset-0" />
      <p className="relative font-medium">Перетащите PCAP / PCAPNG сюда</p>
      <p className="relative text-xs text-muted">Ethernet и TCP/IPv4. Исходный файл сохраняется без изменений.</p>
      <div className="relative mt-2">
        <UploadCaptureButton projectId={projectId} />
      </div>
    </div>
  );
}
