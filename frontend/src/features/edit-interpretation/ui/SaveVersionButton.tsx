import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useState, type FormEvent } from 'react';
import { interpretationApi, type InterpretationSpec, type InterpretationVersion } from '@/entities/interpretation';
import { projectKeys } from '@/entities/project';
import { ApiError } from '@/shared/api';
import { Button, Note, Field, Input, Modal, toast } from '@/shared/ui';

export interface SaveVersionButtonProps {
  projectId: string;
  interpretationId: string;
  spec: InterpretationSpec | null;
  disabled?: boolean;
  onSaved: (version: InterpretationVersion) => void;
}

/** Versions are immutable: saving always creates a new version; old runs become stale. */
export function SaveVersionButton({ projectId, interpretationId, spec, disabled, onSaved }: SaveVersionButtonProps) {
  const [open, setOpen] = useState(false);
  const [comment, setComment] = useState('');
  const qc = useQueryClient();

  const mutation = useMutation({
    mutationFn: () => interpretationApi.createVersion(interpretationId, { spec: spec!, comment }),
    onSuccess: (version) => {
      qc.invalidateQueries({ queryKey: ['interpretation', interpretationId] });
      qc.invalidateQueries({ queryKey: projectKeys.root(projectId) });
      toast.success(`Сохранена версия v${version.version}. Предыдущие проверки помечены как устаревшие.`);
      setOpen(false);
      setComment('');
      onSaved(version);
    },
    onError: (err) => toast.fromError(err),
  });

  const submit = (e: FormEvent) => {
    e.preventDefault();
    mutation.mutate();
  };

  const issues = mutation.error instanceof ApiError && Array.isArray(mutation.error.details) ? mutation.error.details : [];

  return (
    <>
      <Button variant="primary" disabled={disabled || !spec} onClick={() => setOpen(true)}>
        Сохранить версию
      </Button>
      <Modal
        open={open}
        onClose={() => setOpen(false)}
        title="Новая версия интерпретации"
        description="Версия неизменяема и получает собственный хеш. Результаты проверок прошлых версий не выдаются за актуальные."
        footer={
          <>
            <Button variant="ghost" onClick={() => setOpen(false)}>
              Отмена
            </Button>
            <Button variant="primary" type="submit" form="save-version" loading={mutation.isPending}>
              Сохранить
            </Button>
          </>
        }
      >
        <form id="save-version" onSubmit={submit} className="flex flex-col gap-3">
          <Field label="Что изменилось и почему" hint="Ссылка на гипотезу/контрпример помогает следующему инженеру">
            <Input autoFocus value={comment} onChange={(e) => setComment(e.target.value)} placeholder="value как i16 LE (контрпример: интервал 600)" />
          </Field>
          {issues.length > 0 && (
            <Note tone="danger" title="Сервер отклонил спецификацию">
              <ul className="mt-1 list-inside list-disc">
                {(issues as { path: string; message: string }[]).map((i, n) => (
                  <li key={n}>
                    <code className="font-mono text-[11px]">{i.path}</code> {i.message}
                  </li>
                ))}
              </ul>
            </Note>
          )}
        </form>
      </Modal>
    </>
  );
}
