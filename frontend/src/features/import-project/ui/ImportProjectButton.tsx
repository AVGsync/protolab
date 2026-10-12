import { useMutation, useQueryClient } from '@tanstack/react-query';
import { FolderInput } from 'lucide-react';
import { useNavigate } from 'react-router';
import { jobKeys } from '@/entities/job';
import { projectApi, projectKeys } from '@/entities/project';
import { routes } from '@/shared/config';
import { FileButton, toast } from '@/shared/ui';

/** Opens a project bundle moved from another directory or machine. */
export function ImportProjectButton() {
  const qc = useQueryClient();
  const navigate = useNavigate();
  const mutation = useMutation({
    mutationFn: projectApi.import,
    onSuccess: (project) => {
      qc.invalidateQueries({ queryKey: projectKeys.list() });
      qc.invalidateQueries({ queryKey: jobKeys.all });
      toast.success(`Проект «${project.name}» импортируется`);
      navigate(routes.project(project.id));
    },
    onError: (err) => toast.fromError(err),
  });

  return (
    <FileButton accept=".zip,.json" icon={<FolderInput size={14} />} loading={mutation.isPending} onFile={(f) => mutation.mutate(f)}>
      Открыть проект
    </FileButton>
  );
}
