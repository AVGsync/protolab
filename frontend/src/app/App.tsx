import { RouterProvider } from 'react-router';
import { QueryProvider } from './providers/QueryProvider';
import { router } from './router/router';

export function App() {
  return (
    <QueryProvider>
      <RouterProvider router={router} />
    </QueryProvider>
  );
}
