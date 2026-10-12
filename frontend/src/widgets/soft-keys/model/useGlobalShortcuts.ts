import { useEffect } from 'react';
import { useNavigate, useParams } from 'react-router';
import { PROJECT_SECTIONS } from '@/shared/config';
import { isTypingTarget, matchesKey } from './keys';
import { useSoftKeysStore } from './store';

/** Section hotkeys (1-6), the help overlay (?) and the page actions registered via useContextKeys. */
export function useGlobalShortcuts(): void {
  const navigate = useNavigate();
  const { projectId } = useParams();

  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.ctrlKey || e.metaKey || e.altKey || e.repeat || isTypingTarget(e.target)) return;
      const store = useSoftKeysStore.getState();

      if (matchesKey(e, '?')) {
        e.preventDefault();
        store.setHelpOpen(!store.helpOpen);
        return;
      }
      // Any other dialog (including the help overlay) owns the keyboard while it is open.
      if (store.helpOpen || document.querySelector('[role="dialog"]')) return;

      const context = store.groups.flatMap((g) => g.keys).find((k) => !k.disabled && matchesKey(e, k.key));
      if (context) {
        e.preventDefault();
        context.onPress();
        return;
      }
      const section = projectId ? PROJECT_SECTIONS.find((s) => matchesKey(e, s.key)) : undefined;
      if (section && projectId) {
        e.preventDefault();
        navigate(section.to(projectId));
      }
    };
    document.addEventListener('keydown', onKeyDown);
    return () => document.removeEventListener('keydown', onKeyDown);
  }, [navigate, projectId]);
}
