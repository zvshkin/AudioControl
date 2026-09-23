import React from 'react';
import { WindowMinimise } from '../../wailsjs/runtime/runtime';
import { HideToTray } from '../../wailsjs/go/main/App';
import { useI18n } from '../i18n';

export const Titlebar: React.FC = () => {
  const { t } = useI18n();

  return (
    <header className="window-drag h-10 w-full bg-bg-main/80 backdrop-blur-md border-b border-border-subtle flex items-center justify-between px-4 z-50">
      <div className="flex items-center space-x-2">
        <div className="w-2.5 h-2.5 rounded-full bg-accent-cyan animate-pulse-soft shadow-glow" />
        <span className="font-mono text-xs font-semibold uppercase tracking-widest text-gray-400">
          AudioControl <span className="text-accent-cyan">Central</span>
        </span>
      </div>

      {}
      <div className="window-no-drag flex items-center space-x-1">
        <button
          onClick={() => WindowMinimise()}
          className="w-7 h-7 flex items-center justify-center rounded hover:bg-bg-hover text-gray-400 hover:text-white transition-colors duration-150"
          title={t('titlebar.minimize')}
        >
          &#8722;
        </button>
        <button
          onClick={() => HideToTray()}
          className="w-7 h-7 flex items-center justify-center rounded hover:bg-accent-red/20 text-gray-400 hover:text-accent-red transition-colors duration-150"
          title={t('titlebar.hide')}
        >
          &#10005;
        </button>
      </div>
    </header>
  );
};
