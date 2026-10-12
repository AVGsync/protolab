import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useState, type FormEvent } from 'react';
import { useNavigate } from 'react-router';
import { interpretationApi } from '@/entities/interpretation';
import { projectKeys } from '@/entities/project';
import { routes } from '@/shared/config';
import { Button, Field, Input, Modal, Textarea, toast, type ButtonVariant } from '@/shared/ui';
import { STARTER_SPEC } from '../model/useSpecDraft';

export function CreateInterpretationButton({
  projectId,
  variant = 'primary',
  label = 'Новая интерпретация',
}: {
  projectId: string;
  variant?: ButtonVariant;
  label?: string;
}) {
  const [open, setOpen] = useState(false);
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const qc = useQueryClient();
  const navigate = useNavigate();

  const mutation = useMutation({
    mutationFn: () => interpretationApi.create(projectId, { name, description, spec: { ...STARTER_SPEC, protocol: name || STARTER_SPEC.protocol } }),
    onSuccess: (interp) => {
      qc.invalidateQueries({ queryKey: projectKeys.root(projectId) });
      setOpen(false);
      navigate(routes.spec(projectId, { interpretationId: interp.id }));
    },
    onError: (err) => toast.fromError(err),
  });

  const submit = (e: FormEvent) => {
    e.preventDefault();
    mutation.mutate();
  };

  return (
    <>
      <Button variant={variant} onClick={() => setOpen(true)}>
        {label}
      </Button>
      <Modal
        open={open}
        onClose={() => setOpen(false)}
        title="Новая интерпретация"
        footer={
          <>
            <Button variant="ghost" onClick={() => setOpen(false)}>
              Отмена
            </Button>
            <Button variant="primary" type="submit" form="create-interp" loading={mutation.isPending} disabled={!name.trim()}>
              Создать
            </Button>
          </>
        }
      >
        <form id="create-interp" onSubmit={submit} className="flex flex-col gap-4">
          <Field label="Название протокола">
            <Input autoFocus value={name} onChange={(e) => setName(e.target.value)} placeholder="ThermoLink" />
          </Field>
          <Field label="Описание">
            <Textarea value={description} onChange={(e) => setDescription(e.target.value)} />
          </Field>
        </form>
      </Modal>
    </>
  );
}
