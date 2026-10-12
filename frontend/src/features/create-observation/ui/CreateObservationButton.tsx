import { useMutation, useQueryClient, useQuery } from '@tanstack/react-query';
import { NotebookPen } from 'lucide-react';
import { useState, type FormEvent } from 'react';
import { HypothesisStatusBadge, hypothesisQueries } from '@/entities/hypothesis';
import { observationApi } from '@/entities/observation';
import { projectKeys } from '@/entities/project';
import type { SourceRef } from '@/shared/api';
import { formatRange, shortHash } from '@/shared/lib';
import { Button, Callout, Checkbox, Field, Input, KeyValue, Modal, Mono, Textarea, toast } from '@/shared/ui';

export interface CreateObservationButtonProps {
  projectId: string;
  source: SourceRef;
  /** Hex of the selected bytes — shown to the researcher and stored as a snapshot by the backend. */
  previewHex: string;
  hasUnknownBytes?: boolean;
}

/** Records a note bound to exact source bytes (capture + session + direction + range). */
export function CreateObservationButton({ projectId, source, previewHex, hasUnknownBytes }: CreateObservationButtonProps) {
  const [open, setOpen] = useState(false);
  const [note, setNote] = useState('');
  const [tags, setTags] = useState('');
  const [hypothesisIds, setHypothesisIds] = useState<string[]>([]);
  const qc = useQueryClient();
  const hypotheses = useQuery({ ...hypothesisQueries.list(projectId), enabled: open });

  const mutation = useMutation({
    mutationFn: () =>
      observationApi.create(projectId, {
        note,
        source,
        hypothesisIds,
        tags: tags.split(',').map((t) => t.trim()).filter(Boolean),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: projectKeys.root(projectId) });
      toast.success('Наблюдение сохранено с привязкой к источнику');
      setOpen(false);
      setNote('');
      setTags('');
      setHypothesisIds([]);
    },
    onError: (err) => toast.fromError(err),
  });

  const submit = (e: FormEvent) => {
    e.preventDefault();
    mutation.mutate();
  };

  const toggle = (id: string) =>
    setHypothesisIds((ids) => (ids.includes(id) ? ids.filter((x) => x !== id) : [...ids, id]));

  return (
    <>
      <Button size="sm" variant="primary" icon={<NotebookPen size={13} />} onClick={() => setOpen(true)}>
        Наблюдение
      </Button>
      <Modal
        open={open}
        onClose={() => setOpen(false)}
        title="Новое наблюдение"
        description="Наблюдение фиксирует факт в данных и навсегда связано с этими байтами исходной записи."
        size="lg"
        footer={
          <>
            <Button variant="ghost" onClick={() => setOpen(false)}>
              Отмена
            </Button>
            <Button variant="primary" type="submit" form="create-observation" loading={mutation.isPending} disabled={!note.trim()}>
              Сохранить
            </Button>
          </>
        }
      >
        <form id="create-observation" onSubmit={submit} className="flex flex-col gap-4">
          <div className="rounded-md border border-line bg-bg p-3">
            <KeyValue
              items={[
                ['Источник', <Mono key="s">{`${source.sessionId} / ${source.direction}`}</Mono>],
                ['Диапазон', <Mono key="r">{formatRange(source)}</Mono>],
                ['Пакеты', <Mono key="p">#{source.packetNumbers.join(', #') || '—'}</Mono>],
                ['Запись', <Mono key="c">{`${source.captureId} · sha256 ${shortHash(source.captureSha256, 16)}…`}</Mono>],
                ['Байты', <Mono key="b" className="break-all text-glow">{previewHex}</Mono>],
              ]}
            />
          </div>
          {hasUnknownBytes && (
            <Callout tone="warning" title="Диапазон содержит неизвестные байты">
              Часть выделения приходится на пропуск в записи — они не будут интерпретированы как нули.
            </Callout>
          )}
          <Field label="Что наблюдается">
            <Textarea
              autoFocus
              value={note}
              onChange={(e) => setNote(e.target.value)}
              placeholder="После действия «задать 21» здесь значение 0x15…"
            />
          </Field>
          <Field label="Теги" hint="Через запятую">
            <Input value={tags} onChange={(e) => setTags(e.target.value)} placeholder="значение, уставка" />
          </Field>
          <div>
            <p className="mb-2 text-xs font-medium text-muted">Связать с гипотезами</p>
            <div className="flex max-h-48 flex-col gap-1.5 overflow-y-auto">
              {hypotheses.data?.items.map((h) => (
                <div key={h.id} className="flex items-center justify-between gap-3">
                  <Checkbox label={h.title} checked={hypothesisIds.includes(h.id)} onChange={() => toggle(h.id)} />
                  <HypothesisStatusBadge status={h.status} />
                </div>
              ))}
              {hypotheses.data?.items.length === 0 && <p className="text-xs text-subtle">Гипотез пока нет.</p>}
            </div>
          </div>
        </form>
      </Modal>
    </>
  );
}
