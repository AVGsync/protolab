import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useState, type FormEvent } from 'react';
import { projectApi, projectKeys, type ChecksumPolicy, type OverlapPolicy, type Project } from '@/entities/project';
import { Button, Note, Field, Input, Modal, Select, toast } from '@/shared/ui';

/** Reassembly settings are part of reproducibility: changing them marks runs as stale. */
export function ProjectSettingsButton({ project }: { project: Project }) {
  const [open, setOpen] = useState(false);
  const [overlapPolicy, setOverlap] = useState<OverlapPolicy>(project.settings.overlapPolicy);
  const [checksumPolicy, setChecksum] = useState<ChecksumPolicy>(project.settings.checksumPolicy);
  const [ports, setPorts] = useState(project.settings.targetPorts.join(', '));
  const qc = useQueryClient();

  const mutation = useMutation({
    mutationFn: () =>
      projectApi.updateSettings(project.id, {
        overlapPolicy,
        checksumPolicy,
        targetPorts: ports.split(/[\s,]+/).map(Number).filter((p) => Number.isInteger(p) && p > 0),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: projectKeys.root(project.id) });
      qc.invalidateQueries({ queryKey: ['session'] });
      qc.invalidateQueries({ queryKey: ['run'] });
      toast.success('Настройки сохранены');
      setOpen(false);
    },
    onError: (err) => toast.fromError(err),
  });

  const submit = (e: FormEvent) => {
    e.preventDefault();
    mutation.mutate();
  };

  return (
    <>
      <Button size="sm" variant="ghost" onClick={() => setOpen(true)}>
        Настройки
      </Button>
      <Modal
        open={open}
        onClose={() => setOpen(false)}
        title="Параметры восстановления потоков"
        footer={
          <>
            <Button variant="ghost" onClick={() => setOpen(false)}>
              Отмена
            </Button>
            <Button variant="primary" type="submit" form="project-settings" loading={mutation.isPending}>
              Сохранить
            </Button>
          </>
        }
      >
        <form id="project-settings" onSubmit={submit} className="flex flex-col gap-4">
          <Field label="Различающиеся данные в перекрытии" hint="Какую копию байтов использовать при повторной передаче с другими данными">
            <Select value={overlapPolicy} onChange={(e) => setOverlap(e.target.value as OverlapPolicy)}>
              <option value="first">Первый полученный пакет</option>
              <option value="last">Последний полученный пакет</option>
            </Select>
          </Field>
          <Field label="Неверная контрольная сумма TCP" hint="При локальном захвате часто вызвана checksum offloading">
            <Select value={checksumPolicy} onChange={(e) => setChecksum(e.target.value as ChecksumPolicy)}>
              <option value="warn">Использовать данные, показывать предупреждение</option>
              <option value="ignore">Использовать данные без предупреждения</option>
            </Select>
          </Field>
          <Field label="Порты устройства" hint="Сессии на этих портах считаются целевыми">
            <Input value={ports} onChange={(e) => setPorts(e.target.value)} className="font-mono" />
          </Field>
          <Note tone="warn">Изменение параметров пометит существующие проверки как устаревшие.</Note>
        </form>
      </Modal>
    </>
  );
}
