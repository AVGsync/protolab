import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { env } from '@/shared/config';
import { App } from './App';
import './styles/fonts.css';
import './styles/index.css';

/** In mock mode the API is served by an MSW service worker (src/mocks). */
async function enableMocks(): Promise<void> {
  if (!env.useMocks) return;
  const { worker } = await import('@/mocks/browser');
  await worker.start({
    onUnhandledRequest: 'bypass',
    quiet: true,
    serviceWorker: { url: `${import.meta.env.BASE_URL}mockServiceWorker.js` },
  });
}

enableMocks().then(() => {
  createRoot(document.getElementById('root')!).render(
    <StrictMode>
      <App />
    </StrictMode>,
  );
});
