import { IconContext } from '@phosphor-icons/react';
import { RouterProvider } from 'react-router';
import { QueryProvider } from './providers/QueryProvider';
import { router } from './router/router';

/** Icons are thin (light weight) and 16px everywhere; use them sparingly, text labels carry the UI. */
const ICONS = { weight: 'light', size: 16 } as const;

export function App() {
  return (
    <IconContext.Provider value={ICONS}>
      <QueryProvider>
        <RouterProvider router={router} />
      </QueryProvider>
    </IconContext.Provider>
  );
}
