import React, { useEffect, useState } from 'react';
import {
  GetConfig,
  SetGlobalHotkeysEnabled,
  SetLanguage,
  SetAutostart,
  SetStartMinimized,
  IsAutostartEnabled,
  GetHotkeyStatus,
} from '../../wailsjs/go/main/App';
import { hotkey } from '../../wailsjs/go/models';
import { EventsOn } from '../../wailsjs/runtime/runtime';
import { useToast } from './Toast';
import { useI18n, Lang, actionLabel } from '../i18n';

interface ReleaseInfo {
  tag_name: string;
  name: string;
  published_at: string;
  html_url: string;
}

const Toggle: React.FC<{
  checked: boolean;
  disabled?: boolean;
  onChange: () => void;
}> = ({ checked, disabled, onChange }) => (
  <button
    onClick={onChange}
    disabled={disabled}
    role="switch"
    aria-checked={checked}
    className={`relative w-11 h-6 rounded-full transition-colors duration-200 disabled:opacity-50 focus:outline-none focus:ring-2 focus:ring-accent-cyan/50 flex-shrink-0 ${
      checked ? 'bg-accent-cyan/70' : 'bg-gray-700'
    }`}
  >
    <span
      className={`absolute top-0.5 left-0.5 w-5 h-5 bg-white rounded-full shadow transition-transform duration-200 ${
        checked ? 'translate-x-5' : 'translate-x-0'
      }`}
    />
  </button>
);

const Section: React.FC<{ title: string; children: React.ReactNode }> = ({ title, children }) => (
  <div className="bg-bg-hover/40 border border-border-subtle rounded-xl p-5 space-y-4 transition-colors duration-180 hover:border-gray-600">
    <h2 className="text-sm font-semibold text-white uppercase font-mono tracking-wider">{title}</h2>
    {children}
  </div>
);

export const SettingsScreen: React.FC = () => {
  const [loading, setLoading] = useState(false);
  const [latestRelease, setLatestRelease] = useState<ReleaseInfo | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [hotkeysEnabled, setHotkeysEnabled] = useState(true);
  const [autostart, setAutostartState] = useState(false);
  const [startMinimized, setStartMinimizedState] = useState(false);
  const [busy, setBusy] = useState(false);
  const [statuses, setStatuses] = useState<hotkey.RegistrationStatus[]>([]);
  const { show } = useToast();
  const { t, lang, setLang } = useI18n();

  const currentVersion = 'v1.0.0';
  const githubRepo = 'zvshkin/AudioControl'; 

  useEffect(() => {
    GetConfig()
      .then((cfg) => {
        setHotkeysEnabled(!cfg.global_hotkeys_disabled);
        setStartMinimizedState(!!cfg.start_minimized);
      })
      .catch(() => {});

    
    
    IsAutostartEnabled()
      .then(setAutostartState)
      .catch(() => {});

    refreshStatuses();
  }, []);

  
  
  
  useEffect(() => {
    const off = EventsOn('hotkeys:global-toggled', (enabled: boolean) => {
      setHotkeysEnabled(!!enabled);
      refreshStatuses();
    });
    return off;
    
  }, []);

  const refreshStatuses = () => {
    GetHotkeyStatus()
      .then((list) => setStatuses(list || []))
      .catch(() => {});
  };

  const toggleGlobalHotkeys = () => {
    const next = !hotkeysEnabled;
    setBusy(true);
    SetGlobalHotkeysEnabled(next)
      .then(() => {
        setHotkeysEnabled(next);
        show(next ? 'success' : 'info', next ? t('toast.hotkeysOn') : t('toast.hotkeysOff'));
        refreshStatuses();
      })
      .catch((err) => show('error', t('toast.errHotkeysToggle'), String(err)))
      .finally(() => setBusy(false));
  };

  const toggleAutostart = () => {
    const next = !autostart;
    setBusy(true);
    SetAutostart(next)
      .then(() => {
        setAutostartState(next);
        show('success', next ? t('toast.autostartOn') : t('toast.autostartOff'));
      })
      .catch((err) => show('error', t('toast.errAutostart'), String(err)))
      .finally(() => setBusy(false));
  };

  const toggleStartMinimized = () => {
    const next = !startMinimized;
    setBusy(true);
    SetStartMinimized(next)
      .then(() => {
        setStartMinimizedState(next);
        show('success', next ? t('toast.startMinimizedOn') : t('toast.startMinimizedOff'));
      })
      .catch((err) => show('error', t('toast.errAutostart'), String(err)))
      .finally(() => setBusy(false));
  };

  const changeLanguage = (next: Lang) => {
    if (next === lang) return;
    setBusy(true);
    SetLanguage(next)
      .then(() => {
        setLang(next); 
        show('success', t('toast.langChanged'));
      })
      .catch((err) => show('error', t('toast.errLang'), String(err)))
      .finally(() => setBusy(false));
  };

  
  
  const isNewerVersion = (latest: string, current: string): boolean => {
    const parse = (v: string) =>
      v.replace(/^v/i, '').split('.').map((n) => parseInt(n, 10) || 0);
    const [lMajor, lMinor, lPatch] = parse(latest);
    const [cMajor, cMinor, cPatch] = parse(current);
    if (lMajor !== cMajor) return lMajor > cMajor;
    if (lMinor !== cMinor) return lMinor > cMinor;
    return lPatch > cPatch;
  };

  const checkUpdates = async () => {
    setLoading(true);
    setError(null);
    try {
      const response = await fetch(`https://api.github.com/repos/${githubRepo}/releases/latest`);
      if (!response.ok) throw new Error('Failed to fetch release data');
      const data: ReleaseInfo = await response.json();
      setLatestRelease(data);
    } catch (err: any) {
      setError(err.message || 'Network error');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="p-6 max-w-2xl mx-auto space-y-6 text-gray-300">
      <div className="border-b border-border-subtle pb-4">
        <h1 className="text-xl font-bold text-white tracking-wide">{t('settings.title')}</h1>
        <p className="text-xs text-gray-400 mt-1">{t('settings.subtitle')}</p>
      </div>

      <Section title={t('settings.interface')}>
        <div className="flex items-center justify-between">
          <p className="text-sm text-gray-200">{t('settings.language')}</p>
          <div className="flex items-center bg-bg-main border border-border-subtle rounded-lg overflow-hidden">
            {(['ru', 'en'] as const).map((code) => (
              <button
                key={code}
                onClick={() => changeLanguage(code)}
                disabled={busy}
                className={`px-3 py-1.5 text-xs font-mono uppercase transition-colors duration-150 disabled:opacity-50 ${
                  lang === code
                    ? 'bg-accent-cyan/20 text-accent-cyan'
                    : 'text-gray-400 hover:text-white hover:bg-bg-hover'
                }`}
              >
                {code === 'ru' ? 'Русский' : 'English'}
              </button>
            ))}
          </div>
        </div>
      </Section>

      <Section title={t('settings.hotkeys')}>
        <div className="flex items-center justify-between">
          <div className="pr-4">
            <p className="text-sm text-gray-200">{t('settings.globalHotkeys')}</p>
            <p className="text-xs text-gray-500 mt-0.5">{t('settings.globalHotkeysHint')}</p>
          </div>
          <Toggle checked={hotkeysEnabled} disabled={busy} onChange={toggleGlobalHotkeys} />
        </div>

        <div className="pt-3 border-t border-border-subtle space-y-2">
          <div className="flex items-center justify-between">
            <p className="text-xs text-gray-400 font-mono uppercase">{t('settings.diagnostics')}</p>
            <button
              onClick={refreshStatuses}
              className="text-[11px] px-2 py-1 rounded text-gray-500 hover:text-accent-cyan hover:bg-accent-cyan/10 transition-colors duration-150"
            >
              {t('settings.diagnosticsRefresh')}
            </button>
          </div>

          {statuses.length === 0 ? (
            <p className="text-xs text-gray-500">{t('settings.diagnosticsEmpty')}</p>
          ) : (
            <div className="space-y-1">
              {statuses.map((s, i) => (
                <div
                  key={`${s.profile_id}-${s.action}-${i}`}
                  className="flex items-center justify-between text-xs py-1 px-2 rounded bg-bg-main/50"
                >
                  <span className="truncate text-gray-300 pr-2">
                    {s.profile_name} · {actionLabel(t, s.action)}
                  </span>
                  <span className="flex items-center space-x-2 flex-shrink-0">
                    <span className="font-mono text-gray-400">{s.key_combo}</span>
                    <span
                      className={
                        s.registered
                          ? 'text-accent-cyan font-mono'
                          : 'text-accent-red font-mono'
                      }
                      title={s.error || ''}
                    >
                      {s.registered ? t('settings.registered') : t('settings.notRegistered')}
                    </span>
                  </span>
                </div>
              ))}
            </div>
          )}
        </div>
      </Section>

      <Section title={t('settings.startup')}>
        <div className="flex items-center justify-between">
          <div className="pr-4">
            <p className="text-sm text-gray-200">{t('settings.autostart')}</p>
            <p className="text-xs text-gray-500 mt-0.5">{t('settings.autostartHint')}</p>
          </div>
          <Toggle checked={autostart} disabled={busy} onChange={toggleAutostart} />
        </div>

        <div className="flex items-center justify-between pt-3 border-t border-border-subtle">
          <div className="pr-4">
            <p className="text-sm text-gray-200">{t('settings.startMinimized')}</p>
            <p className="text-xs text-gray-500 mt-0.5">{t('settings.startMinimizedHint')}</p>
          </div>
          <Toggle checked={startMinimized} disabled={busy} onChange={toggleStartMinimized} />
        </div>
      </Section>

      <Section title={t('settings.updates')}>
        <div className="flex items-center justify-between">
          <div className="flex items-center space-x-2 text-xs font-mono">
            <span className="text-gray-500">{t('settings.currentVersion')}</span>
            <span className="text-accent-cyan font-bold">{currentVersion}</span>
          </div>
          <button
            onClick={checkUpdates}
            disabled={loading}
            className="px-3 py-1.5 bg-bg-main border border-border-subtle text-xs text-gray-200 rounded-lg hover:border-accent-cyan active:scale-[0.97] transition-all duration-150 disabled:opacity-50"
          >
            {loading ? t('settings.checking') : t('settings.checkUpdates')}
          </button>
        </div>

        {error && <p className="text-xs text-accent-red font-mono animate-row-in">{error}</p>}

        {latestRelease && (
          <div className="p-3 bg-bg-main border border-border-subtle rounded-lg space-y-2 text-xs animate-row-in">
            <div className="flex justify-between items-center">
              <span className="font-bold text-white">
                {latestRelease.name || latestRelease.tag_name}
              </span>
              <span className="text-gray-500 font-mono">
                {new Date(latestRelease.published_at).toLocaleDateString()}
              </span>
            </div>
            {!isNewerVersion(latestRelease.tag_name, currentVersion) ? (
              <p className="text-accent-cyan">{t('settings.upToDate')}</p>
            ) : (
              <p className="text-yellow-400">
                {t('settings.newVersion', { version: latestRelease.tag_name })}{' '}
                <a
                  href={latestRelease.html_url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="underline font-bold"
                >
                  {t('settings.download')}
                </a>
              </p>
            )}
          </div>
        )}
      </Section>

      <Section title={t('settings.support')}>
        <a
          href="https://t.me/zvshkin"
          target="_blank"
          rel="noopener noreferrer"
          className="inline-flex items-center space-x-2 text-xs text-accent-cyan hover:underline"
        >
          <span>{t('settings.supportLink')}</span>
          <span>&#8599;</span>
        </a>
      </Section>

      <Section title={t('settings.about')}>
        <p className="text-xs leading-relaxed text-gray-400">{t('settings.aboutText')}</p>
      </Section>
    </div>
  );
};
