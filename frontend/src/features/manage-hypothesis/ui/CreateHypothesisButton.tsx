import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useState, type FormEvent } from 'react';
import { hypothesisApi, type CreateHypothesisInput } from '@/entities/hypothesis';
import { projectKeys } from '@/entities/project';
import { Button, type ButtonProps, Field, Input, Modal, Textarea, toast } from '@/shared/ui';

export interface CreateHypothesisButtonProps {
  projectId: string;
  defaults?: Partial<CreateHypothesisInput>;
  label?: string;
  size?: ButtonProps['size'];
  variant?: ButtonProps['variant'];
  onCreated?: (id: string) => void;
}

export function CreateHypothesisButton({
  projectId,
  defaults,
  label = 'Новая гипотеза',
  size = 'md',
  variant = 'primary',
  onCreated,
}: CreateHypothesisButtonProps) {
  const [open, setOpen] = useState(false);
  const [title, setTitle] = useState(defaults?.title ?? '');
  const [statement, setStatement] = useState(defaults?.statement ?? '');
  const [alternatives, setAlternatives] = useState((defaults?.alternatives ?? []).join('\n'));
  const qc = useQueryClient();

  const reset = () => {
    setTitle(defaults?.title ?? '');
    setStatement(defaults?.statement ?? '');
    setAlternatives((defaults?.alternatives ?? []).join('\n'));
  };

  const mutation = useMutation({
    mutationFn: () =>
      hypothesisApi.create(projectId, {
        ...defaults,
        title,
        statement,
        alternatives: alternatives.split('\n').map((s) => s.trim()).filter(Boolean),
      }),
    onSuccess: (h) => {
      qc.invalidateQueries({ queryKey: projectKeys.root(projectId) });
      toast.success('Гипотеза создана — проверьте её на других записях');
      setOpen(false);
      onCreated?.(h.id);
    },
    onError: (err) => toast.fromError(err),
  });

  const submit = (e: FormEvent) => {
    e.preventDefault();
    mutation.mutate();
  };

  return (
    <>
      <Button
        size={size}
        variant={variant}
        onClick={() => {
          reset();
          setOpen(true);
        }}
      >
        {label}
      </Button>
      <Modal
        open={open}
        onClose={() => setOpen(false)}
        title="Новая гипотеза"
        description="Гипотеза — предположение о структуре или назначении данных. Совпадение на примерах её не доказывает."
        footer={
          <>
            <Button variant="ghost" onClick={() => setOpen(false)}>
              Отмена
            </Button>
            <Button variant="primary" type="submit" form="create-hypothesis" loading={mutation.isPending} disabled={!title.trim()}>
              Создать
            </Button>
          </>
        }
      >
        <form id="create-hypothesis" onSubmit={submit} className="flex flex-col gap-4">
          <Field label="Кратко">
            <Input autoFocus value={title} onChange={(e) => setTitle(e.target.value)} placeholder="Байты 6–7 — значение настройки (i16 LE)" />
          </Field>
          <Field label="Формулировка" hint="Проверяемое утверждение: на каких данных и что должно выполняться">
            <Textarea value={statement} onChange={(e) => setStatement(e.target.value)} />
          </Field>
          <Field label="Альтернативные объяснения" hint="По одному на строку">
            <Textarea value={alternatives} onChange={(e) => setAlternatives(e.target.value)} />
          </Field>
          {defaults?.observationIds?.length ? (
            <p className="text-xs text-muted">Будет связана с наблюдениями: {defaults.observationIds.join(', ')}</p>
          ) : null}
        </form>
      </Modal>
    </>
  );
}
