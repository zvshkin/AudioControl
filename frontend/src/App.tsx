import React, { useEffect, useState } from 'react';
import { Titlebar } from './components/Titlebar';
import { ProfilesScreen } from './components/ProfilesScreen';
import { SettingsScreen } from './components/SettingsScreen';
import { EventsOn } from '../wailsjs/runtime/runtime';
import { GetConfig } from '../wailsjs/go/main/App';
import { useToast } from './components/Toast';
import { useI18n, Lang, actionLabel } from './i18n';

export const App: React.FC = () => {
  const [activeTab, setActiveTab] = useState<'profiles' | 'settings'>('profiles');
  const { show } = useToast();
  const { t, setLang } = useI18n();

  
  
  useEffect(() => {
    GetConfig()
      .then((cfg) => {
        if (cfg.language === 'en' || cfg.language === 'ru') {
          setLang(cfg.language as Lang);
        }
      })
      .catch(() => {
        
      });
  }, [setLang]);

  useEffect(() => {
    const offMediaStatus = EventsOn('media:target-status', (payload: any) => {
      if (!payload || payload.available) return;
      show(
        'warning',
        t('toast.mediaUnavailable'),
        t('toast.mediaUnavailableText', {
          process: payload.processName,
          action: actionLabel(t, payload.action),
        })
      );
    });

    
    
    const offActionError = EventsOn('hotkey:action-error', (payload: any) => {
      if (!payload) return;
      show('error', t('toast.actionFailed'), `${payload.profileName}: ${payload.message}`);
    });

    const offRegIssue = EventsOn('hotkeys:registration-issue', (message: string) => {
      show('warning', t('toast.hotkeyRegIssue'), String(message));
    });

    const offToggle = EventsOn('hotkeys:global-toggled', (enabled: boolean) => {
      show(enabled ? 'success' : 'info', enabled ? t('toast.hotkeysOn') : t('toast.hotkeysOff'));
    });

    return () => {
      offMediaStatus();
      offActionError();
      offRegIssue();
      offToggle();
    };
  }, [show, t]);

  const tabClass = (tab: 'profiles' | 'settings') =>
    `text-xs font-mono uppercase tracking-wider py-1 px-3 rounded transition-colors duration-150 ${
      activeTab === tab
        ? 'bg-accent-cyan/20 text-accent-cyan border border-accent-cyan/40'
        : 'text-gray-400 hover:text-white border border-transparent'
    }`;

  return (
    <div className="flex flex-col h-screen bg-bg-main text-white select-none">
      <Titlebar />

      <nav className="flex space-x-4 border-b border-border-subtle px-6 py-2 bg-bg-hover/20">
        <button onClick={() => setActiveTab('profiles')} className={tabClass('profiles')}>
          {t('nav.profiles')}
        </button>
        <button onClick={() => setActiveTab('settings')} className={tabClass('settings')}>
          {t('nav.settings')}
        </button>
      </nav>

      <main className="flex-1 overflow-y-auto">
        {activeTab === 'profiles' ? <ProfilesScreen /> : <SettingsScreen />}
      </main>
    </div>
  );
};

export default App;
