import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useState, type FormEvent } from 'react';
import { useNavigate } from 'react-router';
import { projectApi, projectKeys } from '@/entities/project';
import { routes } from '@/shared/config';
import { Button, Field, Input, Modal, Textarea, toast } from '@/shared/ui';

export function CreateProjectButton() {
  const [open, setOpen] = useState(false);
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [ports, setPorts] = useState('');
  const navigate = useNavigate();
  const qc = useQueryClient();

  const mutation = useMutation({
    mutationFn: projectApi.create,
    onSuccess: (project) => {
      qc.invalidateQueries({ queryKey: projectKeys.list() });
      toast.success(`Проект «${project.name}» создан`);
      setOpen(false);
      navigate(routes.project(project.id));
    },
    onError: (err) => toast.fromError(err),
  });

  const submit = (e: FormEvent) => {
    e.preventDefault();
    const targetPorts = ports
      .split(/[\s,]+/)
      .map(Number)
      .filter((p) => Number.isInteger(p) && p > 0 && p < 65536);
    mutation.mutate({ name, description, targetPorts });
  };

  return (
    <>
      <Button variant="primary" onClick={() => setOpen(true)}>
        Новый проект
      </Button>
      <Modal
        open={open}
        onClose={() => setOpen(false)}
        title="Новый проект исследования"
        footer={
          <>
            <Button variant="ghost" onClick={() => setOpen(false)}>
              Отмена
            </Button>
            <Button variant="primary" type="submit" form="create-project" loading={mutation.isPending} disabled={!name.trim()}>
              Создать
            </Button>
          </>
        }
      >
        <form id="create-project" onSubmit={submit} className="flex flex-col gap-4">
          <Field label="Название">
            <Input autoFocus value={name} onChange={(e) => setName(e.target.value)} placeholder="Контроллер ХХХ, стенд №2" />
          </Field>
          <Field label="Описание">
            <Textarea value={description} onChange={(e) => setDescription(e.target.value)} placeholder="Устройство, клиентское приложение, условия эксперимента" />
          </Field>
          <Field label="Порты устройства" hint="Через запятую. Сессии на этих портах отмечаются как целевые.">
            <Input value={ports} onChange={(e) => setPorts(e.target.value)} placeholder="5020" className="font-mono" />
          </Field>
        </form>
      </Modal>
    </>
  );
}
