import { lazy, StrictMode, Suspense } from 'react';
import { createRoot } from 'react-dom/client';
import './index.css';

// Each HTML file names its page; each page loads as its own chunk, so the landing page carries no workbench code.
const pages = {
  index: lazy(() => import('./landing/Landing.tsx')),
  search: lazy(() => import('./pages/Search.tsx')),
  library: lazy(() => import('./pages/Library.tsx')),
  video: lazy(() => import('./pages/Video.tsx')),
  signin: lazy(() => import('./pages/SignIn.tsx')),
};
const Page = pages[document.body.dataset.page as keyof typeof pages] ?? pages.index;

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <Suspense fallback={null}>
      <Page />
    </Suspense>
  </StrictMode>,
);
