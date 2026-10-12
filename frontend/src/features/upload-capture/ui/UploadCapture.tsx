import { FileUp, UploadCloud } from 'lucide-react';
import { useState, type DragEvent } from 'react';
import { cn } from '@/shared/lib';
import { FileButton } from '@/shared/ui';
import { useUploadCapture } from '../model/useUploadCapture';

const ACCEPT = '.pcap,.pcapng,.cap';

export function UploadCaptureButton({ projectId }: { projectId: string }) {
  const upload = useUploadCapture(projectId);
  return (
    <FileButton variant="primary" accept={ACCEPT} icon={<FileUp size={14} />} loading={upload.isPending} onFile={(f) => upload.mutate(f)}>
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
        'flex flex-col items-center justify-center gap-2 rounded-lg border border-dashed px-6 py-8 text-center transition-colors',
        over ? 'border-accent bg-accent/5' : 'border-line-strong bg-surface/50',
        className,
      )}
    >
      <UploadCloud className={over ? 'text-accent' : 'text-muted'} size={26} />
      <p className="font-medium">Перетащите PCAP / PCAPNG сюда</p>
      <p className="max-w-md text-xs text-muted">
        Ethernet + TCP/IPv4. Исходный файл сохраняется без изменений, его SHA-256 фиксируется. Неподдерживаемые данные диагностируются явно.
      </p>
      <div className="mt-2">
        <UploadCaptureButton projectId={projectId} />
      </div>
    </div>
  );
}
