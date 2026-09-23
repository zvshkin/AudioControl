import React from 'react';
import { createRoot } from 'react-dom/client';
import { App } from './App';
import { ToastProvider } from './components/Toast';
import { I18nProvider } from './i18n';
import { BrowserOpenURL } from '../wailsjs/runtime/runtime';
import './index.css';

document.addEventListener('click', (e) => {
  const anchor = (e.target as HTMLElement | null)?.closest?.('a');
  if (!anchor?.href) return;
  if (!/^https?:/i.test(anchor.href)) return;
  let external: URL;
  try {
    external = new URL(anchor.href);
  } catch {
    return;
  }
  if (external.host === window.location.host) return;
  e.preventDefault();
  BrowserOpenURL(anchor.href);
});

const container = document.getElementById('root');
const root = createRoot(container!);
root.render(
  <React.StrictMode>
    <I18nProvider>
      <ToastProvider>
        <App />
      </ToastProvider>
    </I18nProvider>
  </React.StrictMode>
);