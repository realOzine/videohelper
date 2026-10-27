import { lazy, StrictMode, Suspense, useEffect, useState } from 'react';
import { createRoot } from 'react-dom/client';
import './index.css';
import { connect } from './api.ts';

// Keep page URLs and lazy chunks while sharing the desktop session.
const pages = {
  search: lazy(() => import('./pages/Search.tsx')),
  library: lazy(() => import('./pages/Library.tsx')),
  video: lazy(() => import('./pages/Video.tsx')),
};
const Page = pages[document.body.dataset.page as keyof typeof pages] ?? pages.search;

function App() {
  const [state, setState] = useState<string | null>(null);
  useEffect(() => { connect().then(() => setState('')).catch((err) => setState(String(err.message))); }, []);
  if (state === null) return <main className="grid min-h-svh place-items-center text-fg-soft" role="status">VideoHelper…</main>;
  if (state) return <main className="mx-auto max-w-xl px-6 py-12"><h1 className="disp text-3xl">VideoHelper</h1><p role="alert" className="mt-6 text-neg-ink">{state}</p></main>;
  return <Page />;
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <Suspense fallback={null}>
      <App />
    </Suspense>
  </StrictMode>,
);
