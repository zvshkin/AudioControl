import React, { useEffect, useRef, useState } from 'react';
import {
  GetConfig,
  SaveProfile,
  DeleteProfile,
  DuplicateProfile,
  MoveProfile,
  GetRunningProcesses,
  GetHotkeyConflicts,
  GetHotkeyStatus,
  GetMediaTargets,
  GetMediaTargetsBatch,
  GetProfileStates,
  SetProfileVolume,
  SetProfileMute,
  TestAction,
} from '../../wailsjs/go/main/App';
import { config, process, hotkey, main } from '../../wailsjs/go/models';
import { EventsOn } from '../../wailsjs/runtime/runtime';
import { HotkeyRecorder, HotkeyValue } from './HotkeyRecorder';
import { ProcessModal } from './ProcessModal';
import { useToast } from './Toast';
import { useI18n, ActionType, actionLabel as actionText } from '../i18n';

interface ActionMeta {
  action: ActionType;
  group: 'volume' | 'media';
}



type HotkeyBindingData = Pick<
  config.HotkeyBinding,
  'id' | 'action' | 'key_combo' | 'key_code' | 'modifiers'
>;

type ProcessProfileData = Pick<
  config.ProcessProfile,
  | 'id'
  | 'process_name'
  | 'display_name'
  | 'enabled'
  | 'target_app_user_model_id'
  | 'volume_step_percent'
> & { hotkeys: HotkeyBindingData[] };

const ACTIONS: ActionMeta[] = [
  { action: 'volume_up', group: 'volume' },
  { action: 'volume_down', group: 'volume' },
  { action: 'toggle_mute', group: 'volume' },
  { action: 'play_pause', group: 'media' },
  { action: 'prev_track', group: 'media' },
  { action: 'next_track', group: 'media' },
];

const MEDIA_ACTIONS: ActionType[] = ['play_pause', 'prev_track', 'next_track'];
const TESTABLE_ACTIONS: ActionType[] = ['volume_up', 'volume_down', 'toggle_mute'];
const DEFAULT_STEP = 10;
const STATE_POLL_MS = 3000;
/** Дебаунс сохранения имени профиля: печатаем в черновик, на сервер — после паузы. */
const NAME_SAVE_DEBOUNCE_MS = 400;

/**
 * Сборка кэша иконок из списка запущенных процессов.
 *
 * Почему кэш, а не запрос на каждую карточку: GetRunningProcesses сканирует
 * все процессы и достаёт иконки из exe — на каждой карточке это превратилось
 * бы в лавину одинаковых IPC-вызовов, а иконки нужны все сразу при открытии
 * вкладки. Ключ — lowercase process_name: имя .exe в конфиге и в списке
 * запущенных может отличаться регистром, сравниваем аккуратно.
 */
const buildIcons = (list: process.ProcessInfo[]): Record<string, string> => {
  const map: Record<string, string> = {};
  list.forEach((p) => {
    if (p.icon_base64 && p.process_name) map[p.process_name.toLowerCase()] = p.icon_base64;
  });
  return map;
};

/** Компактный ввод шага громкости: [−][ 10 ][+] %. */
const StepInput: React.FC<{
  value: number;
  onChange: (v: number) => void;
  label: string;
  title: string;
}> = ({ value, onChange, label, title }) => {
  const [draft, setDraft] = useState(String(value));

  useEffect(() => setDraft(String(value)), [value]);

  const clamp = (n: number) => Math.min(100, Math.max(1, n));

  const commit = (raw: string) => {
    const parsed = parseInt(raw, 10);
    if (Number.isNaN(parsed)) {
      setDraft(String(value));
      return;
    }
    const next = clamp(parsed);
    setDraft(String(next));
    if (next !== value) onChange(next);
  };

  const nudge = (delta: number) => {
    const next = clamp(value + delta);
    if (next !== value) onChange(next);
  };

  return (
    <div className="flex items-center space-x-2" title={title}>
      <span className="text-[11px] text-gray-500 font-mono uppercase">{label}</span>
      <div className="flex items-center bg-bg-main border border-border-subtle rounded-lg overflow-hidden focus-within:border-accent-cyan transition-colors duration-150">
        <button
          type="button"
          onClick={() => nudge(-1)}
          className="w-6 h-7 text-gray-400 hover:text-accent-cyan hover:bg-bg-hover active:scale-95 transition-all duration-150"
          tabIndex={-1}
        >
          &#8722;
        </button>
        <input
          type="text"
          inputMode="numeric"
          value={draft}
          onChange={(e) => setDraft(e.target.value.replace(/[^0-9]/g, ''))}
          onBlur={(e) => commit(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') (e.target as HTMLInputElement).blur();
            if (e.key === 'ArrowUp') {
              e.preventDefault();
              nudge(1);
            }
            if (e.key === 'ArrowDown') {
              e.preventDefault();
              nudge(-1);
            }
          }}
          className="w-9 h-7 bg-transparent text-center text-xs font-mono font-semibold text-accent-cyan focus:outline-none"
        />
        <span className="pr-1.5 text-[11px] text-gray-500 font-mono">%</span>
        <button
          type="button"
          onClick={() => nudge(1)}
          className="w-6 h-7 text-gray-400 hover:text-accent-cyan hover:bg-bg-hover active:scale-95 transition-all duration-150 border-l border-border-subtle"
          tabIndex={-1}
        >
          +
        </button>
      </div>
    </div>
  );
};

/**
 * Имя профиля: локальный черновик + отложенное сохранение (debounce).
 *
 * Почему не «обычный» controlled-инпут от конфига с бэка: значение из state
 * обновлялось только после ответа SaveProfile, и при быстром наборе рендер
 * откатывал DOM к устаревшему значению — символы буквально терялись. Плюс
 * каждый символ раньше означал запись конфига на диск И полную
 * перерегистрацию всех хоткеев (последнее чинится на бэке — см.
 * hotkeyRelevantEqual в app.go).
 *
 * Синхронизация с пропсом — только вне фокуса, чтобы внешнее обновление не
 * перетирало текущий набор. Сохранение уходит и по дебаунсу, и на blur
 * (клик по любой кнопке сначала блюрит поле — поэтому «набрал и сразу
 * нажал Удалить/вкладку» не теряет правку, а удалять компонент с
 * несохранённым таймером безопасно: дебаунс просто отменяется).
 */
const ProfileNameInput: React.FC<{
  profile: config.ProcessProfile;
  onSave: (name: string) => void;
}> = ({ profile, onSave }) => {
  const [draft, setDraft] = useState(profile.display_name);
  const draftRef = useRef(profile.display_name);
  const savedRef = useRef(profile.display_name);
  const onSaveRef = useRef(onSave);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const focusedRef = useRef(false);

  
  
  useEffect(() => {
    onSaveRef.current = onSave;
  });

  
  useEffect(() => {
    if (focusedRef.current) return;
    draftRef.current = profile.display_name;
    savedRef.current = profile.display_name;
    setDraft(profile.display_name);
  }, [profile.display_name]);

  
  
  
  useEffect(
    () => () => {
      if (timerRef.current) clearTimeout(timerRef.current);
    },
    []
  );

  const flush = (value: string) => {
    if (value === savedRef.current) return;
    savedRef.current = value;
    onSaveRef.current(value);
  };

  const change = (value: string) => {
    draftRef.current = value;
    setDraft(value);
    if (timerRef.current) clearTimeout(timerRef.current);
    timerRef.current = setTimeout(() => flush(draftRef.current), NAME_SAVE_DEBOUNCE_MS);
  };

  const blur = () => {
    focusedRef.current = false;
    if (timerRef.current) {
      clearTimeout(timerRef.current);
      timerRef.current = null;
    }
    flush(draftRef.current);
  };

  return (
    <input
      type="text"
      value={draft}
      onChange={(e) => change(e.target.value)}
      onFocus={() => {
        focusedRef.current = true;
      }}
      onBlur={blur}
      className="bg-transparent text-sm font-semibold text-white border-b border-transparent hover:border-border-subtle focus:border-accent-cyan focus:outline-none px-1 min-w-0 transition-colors duration-150"
    />
  );
};

/**
 * Живой регулятор громкости. Живёт в секции "Громкость", а не в шапке карточки —
 * там ему самое место рядом с шагом изменения, а не вперемешку с именем профиля,
 * переключателем активности и кнопками действий.
 *
 * Не шлёт запрос на каждое движение мыши: каждый вызов поднимает и опускает COM,
 * поток таких запросов забил бы очередь. Во время перетаскивания показывается
 * локальное значение, на сервер — не чаще раза в ~80мс, и обязательно финальное
 * значение на отпускании.
 */
const VolumeSlider: React.FC<{
  value: number;
  muted: boolean;
  onMuteToggle: () => void;
  onCommit: (v: number) => void;
}> = ({ value, muted, onMuteToggle, onCommit }) => {
  const [local, setLocal] = useState(value);
  const [dragging, setDragging] = useState(false);
  const lastSent = useRef(0);
  const { t } = useI18n();

  useEffect(() => {
    if (!dragging) setLocal(value);
  }, [value, dragging]);

  const shown = dragging ? local : value;

  return (
    <div className="flex items-center space-x-2">
      <button
        onClick={onMuteToggle}
        title={muted ? t('profiles.unmute') : t('profiles.mute')}
        className={`w-7 h-7 rounded flex items-center justify-center flex-shrink-0 transition-colors duration-150 ${
          muted ? 'text-accent-red bg-accent-red/10' : 'text-gray-400 hover:text-accent-cyan hover:bg-accent-cyan/10'
        }`}
      >
        {muted ? '🔇' : '🔊'}
      </button>
      <input
        type="range"
        min={0}
        max={100}
        value={shown}
        onMouseDown={() => setDragging(true)}
        onChange={(e) => {
          const v = Number(e.target.value);
          setLocal(v);
          const now = Date.now();
          if (now - lastSent.current > 80) {
            lastSent.current = now;
            onCommit(v);
          }
        }}
        onMouseUp={(e) => {
          setDragging(false);
          onCommit(Number((e.target as HTMLInputElement).value));
        }}
        onKeyUp={(e) => onCommit(Number((e.target as HTMLInputElement).value))}
        className={`vol-slider flex-1 ${muted ? 'opacity-40' : ''}`}
      />
      <span
        className={`text-xs font-mono w-9 text-right tabular-nums flex-shrink-0 ${
          muted ? 'text-gray-500 line-through' : 'text-accent-cyan'
        }`}
      >
        {shown}%
      </span>
    </div>
  );
};

export const ProfilesScreen: React.FC = () => {
  const [profiles, setProfiles] = useState<config.ProcessProfile[]>([]);
  const [globalHotkeysDisabled, setGlobalHotkeysDisabled] = useState(false);
  const [conflicts, setConflicts] = useState<hotkey.Conflict[]>([]);
  const [statuses, setStatuses] = useState<hotkey.RegistrationStatus[]>([]);
  const [states, setStates] = useState<Record<string, main.ProfileState>>({});
  const [mediaTargets, setMediaTargets] = useState<Record<string, string[]>>({});
  const [processes, setProcesses] = useState<process.ProcessInfo[]>([]);
  
  const [icons, setIcons] = useState<Record<string, string>>({});
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [processPickerTarget, setProcessPickerTarget] = useState<'new' | string | null>(null);
  const [pendingDeleteId, setPendingDeleteId] = useState<string | null>(null);
  const { show } = useToast();
  const { t } = useI18n();

  
  const actionLabel = (a: string) => actionText(t, a);

  useEffect(() => {
    refreshAll();

    
    
    
    GetRunningProcesses()
      .then((list) => setIcons((prev) => ({ ...prev, ...buildIcons(list || []) })))
      .catch(() => {});

    
    
    
    
    const tick = () => {
      if (!document.hidden) refreshStates();
    };
    const timer = setInterval(tick, STATE_POLL_MS);
    const onVisibility = () => {
      if (!document.hidden) refreshStates();
    };
    document.addEventListener('visibilitychange', onVisibility);
    return () => {
      clearInterval(timer);
      document.removeEventListener('visibilitychange', onVisibility);
    };
    
  }, []);

  
  
  
  useEffect(() => {
    const off = EventsOn('hotkeys:global-toggled', (enabled: boolean) => {
      setGlobalHotkeysDisabled(!enabled);
    });
    return off;
  }, []);

  const refreshAll = () => {
    GetConfig()
      .then((cfg) => {
        const list = cfg.profiles || [];
        setProfiles(list);
        setGlobalHotkeysDisabled(!!cfg.global_hotkeys_disabled);

        const mediaProcesses = list
          .filter((p) => (p.hotkeys || []).some((h) => MEDIA_ACTIONS.includes(h.action as ActionType)))
          .map((p) => p.process_name)
          .filter((name): name is string => !!name);

        if (mediaProcesses.length > 0) {
          
          
          GetMediaTargetsBatch(mediaProcesses)
            .then((map) => setMediaTargets((prev) => ({ ...prev, ...(map || {}) })))
            .catch(() => {});
        }
      })
      .catch((err) => show('error', t('toast.errLoad'), String(err)));

    refreshDiagnostics();
    refreshStates();
  };

  const refreshStates = () => {
    GetProfileStates()
      .then((list) => {
        const map: Record<string, main.ProfileState> = {};
        (list || []).forEach((s) => {
          map[s.profile_id] = s;
        });
        setStates(map);
      })
      .catch(() => {});
  };

  const refreshDiagnostics = () => {
    GetHotkeyConflicts().then((l) => setConflicts(l || [])).catch(() => {});
    GetHotkeyStatus().then((l) => setStatuses(l || [])).catch(() => {});
  };

  const refreshMediaTargets = (processName: string) => {
    if (!processName) return;
    GetMediaTargets(processName)
      .then((targets) => setMediaTargets((prev) => ({ ...prev, [processName]: targets || [] })))
      .catch(() => {});
  };

  const applyConfig = (cfg: config.AppConfig, message?: string) => {
    setProfiles(cfg.profiles || []);
    if (message) show('success', message);
    refreshDiagnostics();
  };

  const persistProfile = (profile: ProcessProfileData, successMessage?: string) => {
    SaveProfile(profile as unknown as config.ProcessProfile)
      .then((cfg) => applyConfig(cfg, successMessage))
      .catch((err) => show('error', t('toast.errSave'), String(err)));
  };

  const removeProfile = (id: string, displayName: string) => {
    DeleteProfile(id)
      .then((cfg) => {
        setProfiles(cfg.profiles || []);
        show('success', t('toast.profileDeleted'), displayName);
        refreshDiagnostics();
      })
      .catch((err) => show('error', t('toast.errDelete'), String(err)));
    setPendingDeleteId(null);
  };

  const openProcessModal = (target: 'new' | string) => {
    setProcessPickerTarget(target);
    GetRunningProcesses()
      .then((list) => {
        setProcesses(list || []);
        
        
        setIcons((prev) => ({ ...prev, ...buildIcons(list || []) }));
        setIsModalOpen(true);
      })
      .catch((err) => show('error', t('toast.errProcesses'), String(err)));
  };

  const handleSelectProcess = (proc: process.ProcessInfo) => {
    const name = proc.display_name || proc.process_name;

    if (processPickerTarget === 'new') {
      persistProfile(
        {
          id: '',
          process_name: proc.process_name,
          display_name: name,
          enabled: true,
          hotkeys: [],
          target_app_user_model_id: '',
          volume_step_percent: DEFAULT_STEP,
        },
        t('toast.profileCreated', { name })
      );
    } else if (processPickerTarget) {
      const target = profiles.find((p) => p.id === processPickerTarget);
      if (!target) return;
      persistProfile(
        { ...target, process_name: proc.process_name, display_name: name, target_app_user_model_id: '' },
        t('toast.processChanged', { name })
      );
    }
    setProcessPickerTarget(null);
  };

  const getHotkeyValue = (profile: config.ProcessProfile, action: ActionType): HotkeyValue | null => {
    const hk = (profile.hotkeys || []).find((h) => h.action === action);
    if (!hk || !hk.key_code) return null;
    return { combo: hk.key_combo, keyCode: hk.key_code, modifiers: hk.modifiers };
  };

  const getConflictMessage = (profile: config.ProcessProfile, action: ActionType): string | null => {
    const hk = (profile.hotkeys || []).find((h) => h.action === action);
    if (!hk || !hk.key_code) return null;

    const conflict = conflicts.find((c) => c.key_code === hk.key_code && c.modifiers === hk.modifiers);
    if (!conflict) return null;

    const other = conflict.bindings.find((b) => b.hotkey_id !== hk.id);
    if (!other) return null;

    return t('hotkey.conflict', { profile: other.display_name, action: actionLabel(other.action) });
  };

  const isRejected = (profile: config.ProcessProfile, action: ActionType): boolean => {
    const st = statuses.find((s) => s.profile_id === profile.id && s.action === action);
    return !!st && !st.registered;
  };

  const updateHotkey = (profile: config.ProcessProfile, action: ActionType, value: HotkeyValue | null) => {
    const withoutAction: HotkeyBindingData[] = (profile.hotkeys || []).filter((h) => h.action !== action);

    if (value) {
      withoutAction.push({
        id: (crypto as any).randomUUID ? crypto.randomUUID() : `${Date.now()}-${action}`,
        action,
        key_combo: value.combo,
        key_code: value.keyCode,
        modifiers: value.modifiers,
      });
    }

    persistProfile(
      { ...profile, hotkeys: withoutAction },
      value
        ? t('toast.hotkeySet', { action: actionLabel(action), combo: value.combo })
        : t('toast.hotkeyCleared', { action: actionLabel(action) })
    );

    if (value && MEDIA_ACTIONS.includes(action)) refreshMediaTargets(profile.process_name);
  };

  const changeVolume = (profileID: string, percent: number) => {
    SetProfileVolume(profileID, percent)
      .then((st) => setStates((prev) => ({ ...prev, [profileID]: st })))
      .catch((err) => show('error', t('toast.errVolume'), String(err)));
  };

  const toggleMute = (profileID: string, muted: boolean) => {
    SetProfileMute(profileID, muted)
      .then((st) => setStates((prev) => ({ ...prev, [profileID]: st })))
      .catch((err) => show('error', t('toast.errVolume'), String(err)));
  };

  return (
    <div className="p-6 space-y-5 max-w-4xl mx-auto">
      <div className="flex items-center justify-between border-b border-border-subtle pb-4">
        <div>
          <h1 className="text-xl font-bold text-white tracking-wide">{t('profiles.title')}</h1>
          <p className="text-xs text-gray-400 mt-1">{t('profiles.subtitle')}</p>
        </div>
        <button
          onClick={() => openProcessModal('new')}
          className="px-4 py-2 bg-accent-cyan/10 border border-accent-cyan/40 text-accent-cyan text-xs font-semibold rounded-lg hover:bg-accent-cyan/20 active:scale-[0.97] transition-all duration-150 shadow-glow"
        >
          {t('profiles.add')}
        </button>
      </div>

      {globalHotkeysDisabled && (
        <div className="flex items-start space-x-2 px-3 py-2 bg-yellow-500/10 border border-yellow-500/30 rounded-lg text-xs text-yellow-400 animate-row-in">
          <span className="font-bold">!</span>
          <span>{t('hotkeys.globalOff')}</span>
        </div>
      )}

      <div className="grid grid-cols-1 gap-5">
        {profiles.map((profile, index) => {
          const targets = mediaTargets[profile.process_name] || [];
          const step = profile.volume_step_percent || DEFAULT_STEP;
          const hasMediaHotkeys = (profile.hotkeys || []).some((h) =>
            MEDIA_ACTIONS.includes(h.action as ActionType)
          );
          const isPendingDelete = pendingDeleteId === profile.id;
          const state = states[profile.id];
          const live = !!state?.has_session;
          
          
          const icon = icons[(profile.process_name || '').toLowerCase()];

          return (
            <div
              key={profile.id}
              
              
              
              
              
              className="group relative bg-bg-hover/40 border border-border-subtle rounded-xl p-4 space-y-4 transition-all duration-180 hover:-translate-y-0.5 hover:border-border-glow hover:shadow-glow animate-row-in"
            >
              {}
              <span
                aria-hidden="true"
                className={`absolute inset-y-0 left-0 w-[3px] rounded-l-xl transition-colors duration-150 ${
                  profile.enabled ? 'bg-accent-cyan' : 'bg-white/10'
                }`}
              />
              {}
              <div className="flex items-center justify-between gap-2">
                <div className="flex items-center gap-2.5 min-w-0">
                  {}
                  <div className="relative flex-shrink-0">
                    {icon ? (
                      <img src={icon} alt="" className="w-7 h-7 rounded" />
                    ) : (
                      <div className="w-7 h-7 rounded bg-bg-main border border-border-subtle flex items-center justify-center text-[11px] font-bold uppercase text-accent-cyan">
                        {(profile.display_name || profile.process_name || '?').charAt(0)}
                      </div>
                    )}
                    {}
                    <span
                      title={live ? t('profiles.live') : t('profiles.noSessionHint')}
                      className={`absolute -bottom-0.5 -right-0.5 w-2.5 h-2.5 rounded-full border-2 border-black/50 ${
                        live ? 'bg-accent-cyan animate-pulse-soft shadow-glow' : 'bg-gray-600'
                      }`}
                    />
                  </div>
                  {}
                  <div className={`min-w-0 ${profile.enabled ? '' : 'opacity-50'}`}>
                    <ProfileNameInput
                      profile={profile}
                      onSave={(name) => persistProfile({ ...profile, display_name: name })}
                    />
                  </div>
                </div>

                {}
                <div className="flex items-center gap-1.5 flex-shrink-0 opacity-0 pointer-events-none transition-opacity duration-150 group-hover:opacity-100 group-hover:pointer-events-auto group-focus-within:opacity-100 group-focus-within:pointer-events-auto">
                  {}
                  <button
                    role="switch"
                    aria-checked={profile.enabled}
                    onClick={() =>
                      persistProfile(
                        { ...profile, enabled: !profile.enabled },
                        !profile.enabled
                          ? t('toast.profileEnabled', { name: profile.display_name })
                          : t('toast.profileDisabled', { name: profile.display_name })
                      )
                    }
                    title={profile.enabled ? t('profiles.active') : t('profiles.disabled')}
                    className={`relative w-9 h-5 rounded-full flex-shrink-0 transition-colors duration-150 ${
                      profile.enabled
                        ? 'bg-accent-cyan/25 border border-accent-cyan/50'
                        : 'bg-gray-800 border border-gray-600'
                    }`}
                  >
                    <span
                      className={`absolute top-[3px] left-[3px] w-3.5 h-3.5 rounded-full transition-all duration-150 ${
                        profile.enabled
                          ? 'translate-x-4 bg-accent-cyan shadow-glow'
                          : 'translate-x-0 bg-gray-500'
                      }`}
                    />
                  </button>

                  <button
                    onClick={() => MoveProfile(profile.id, -1).then((c) => applyConfig(c)).catch(() => {})}
                    disabled={index === 0}
                    title={t('profiles.moveUp')}
                    className="w-6 h-6 rounded text-gray-500 hover:text-accent-cyan hover:bg-bg-hover disabled:opacity-25 transition-colors duration-150"
                  >
                    &#9650;
                  </button>
                  <button
                    onClick={() => MoveProfile(profile.id, 1).then((c) => applyConfig(c)).catch(() => {})}
                    disabled={index === profiles.length - 1}
                    title={t('profiles.moveDown')}
                    className="w-6 h-6 rounded text-gray-500 hover:text-accent-cyan hover:bg-bg-hover disabled:opacity-25 transition-colors duration-150"
                  >
                    &#9660;
                  </button>
                  <button
                    onClick={() =>
                      DuplicateProfile(profile.id)
                        .then((c) => applyConfig(c, t('toast.profileDuplicated')))
                        .catch((err) => show('error', t('toast.errSave'), String(err)))
                    }
                    title={t('profiles.duplicate')}
                    className="w-6 h-6 rounded text-gray-500 hover:text-accent-cyan hover:bg-bg-hover transition-colors duration-150"
                  >
                    &#10697;
                  </button>

                  {isPendingDelete ? (
                    <button
                      onClick={() => removeProfile(profile.id, profile.display_name)}
                      onBlur={() => setPendingDeleteId(null)}
                      autoFocus
                      className="text-[10px] px-2 py-1 rounded-full bg-accent-red/20 text-accent-red border border-accent-red/50 font-semibold transition-colors duration-150"
                    >
                      {t('profiles.deleteConfirm')}
                    </button>
                  ) : (
                    <button
                      onClick={() => setPendingDeleteId(profile.id)}
                      title={t('profiles.delete')}
                      className="w-6 h-6 rounded text-gray-500 hover:text-accent-red hover:bg-accent-red/10 transition-colors duration-150"
                    >
                      &#10005;
                    </button>
                  )}
                </div>
              </div>

              <div className="space-y-1">
                <label className="text-[11px] text-gray-400 font-mono uppercase">
                  {t('profiles.process')}
                </label>
                <button
                  onClick={() => openProcessModal(profile.id)}
                  className="w-full px-3 py-2 text-xs text-left bg-bg-main border border-border-subtle rounded-lg text-gray-300 hover:border-gray-500 focus:border-accent-cyan focus:outline-none truncate transition-colors duration-150"
                >
                  {profile.process_name || t('profiles.processPlaceholder')}
                </button>
              </div>

              {hasMediaHotkeys && (
                <div className="space-y-1 animate-row-in">
                  <div className="flex items-center justify-between">
                    <label className="text-[11px] text-gray-400 font-mono uppercase">
                      {t('profiles.mediaTarget')}{' '}
                      {targets.length > 0 && `(${t('profiles.mediaTargetFound', { count: targets.length })})`}
                    </label>
                    <button
                      onClick={() => refreshMediaTargets(profile.process_name)}
                      className="text-[10px] font-mono px-1.5 py-0.5 rounded text-gray-500 hover:text-accent-cyan hover:bg-accent-cyan/10 transition-colors duration-150"
                    >
                      {t('settings.diagnosticsRefresh')}
                    </button>
                  </div>
                  <select
                    value={profile.target_app_user_model_id || ''}
                    onChange={(e) =>
                      persistProfile(
                        { ...profile, target_app_user_model_id: e.target.value },
                        t('toast.mediaTargetChanged')
                      )
                    }
                    className="w-full px-3 py-2 text-xs bg-bg-main border border-border-subtle rounded-lg text-gray-300 focus:border-accent-cyan focus:outline-none"
                  >
                    <option value="">{t('profiles.mediaTargetAuto')}</option>
                    {targets.map((tg) => (
                      <option key={tg} value={tg}>
                        {tg}
                      </option>
                    ))}
                  </select>
                  <p className="text-[11px] text-gray-500">{t('profiles.mediaTargetHint')}</p>
                </div>
              )}

              {(['volume', 'media'] as const).map((group, groupIndex) => (
                <div
                  key={group}
                  
                  
                  className={`space-y-2 ${groupIndex > 0 ? 'border-t border-border-subtle pt-4' : ''}`}
                >
                  <div className="flex items-center justify-between min-h-[28px]">
                    <label className="flex items-center gap-1.5 text-[11px] text-gray-400 font-mono uppercase">
                      {}
                      <span className="text-[13px] leading-none">
                        {group === 'volume' ? '🔉' : '🎵'}
                      </span>
                      {group === 'volume' ? t('group.volume') : t('group.media')}
                    </label>
                    {group === 'volume' && (
                      <StepInput
                        value={step}
                        label={t('profiles.step')}
                        title={t('profiles.stepHint')}
                        onChange={(v) =>
                          persistProfile(
                            { ...profile, volume_step_percent: v },
                            t('toast.stepChanged', { step: v })
                          )
                        }
                      />
                    )}
                  </div>

                  {}
                  {group === 'volume' && live && (
                    <VolumeSlider
                      value={state.volume}
                      muted={state.muted}
                      onMuteToggle={() => toggleMute(profile.id, !state.muted)}
                      onCommit={(v) => changeVolume(profile.id, v)}
                    />
                  )}
                  {group === 'volume' && !live && (
                    <p className="text-[11px] text-gray-500">{t('profiles.noSession')}</p>
                  )}

                  <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
                    {ACTIONS.filter((a) => a.group === group).map((meta) => {
                      const conflictMessage = getConflictMessage(profile, meta.action);
                      const rejected = isRejected(profile, meta.action);

                      return (
                        <div key={meta.action} className="space-y-1">
                          <div className="flex items-center justify-between">
                            <label className="text-[11px] text-gray-500">{actionLabel(meta.action)}</label>
                            {TESTABLE_ACTIONS.includes(meta.action) && (
                              <button
                                onClick={() =>
                                  TestAction(profile.id, meta.action)
                                    .then((result) => {
                                      show('success', t('toast.testOk', { result }));
                                      refreshStates();
                                    })
                                    .catch((err) => show('error', t('toast.actionFailed'), String(err)))
                                }
                                title={t('profiles.testHint')}
                                className="text-[10px] font-mono px-1.5 py-0.5 rounded text-gray-500 hover:text-accent-cyan hover:bg-accent-cyan/10 transition-colors duration-150"
                              >
                                {t('profiles.test')}
                              </button>
                            )}
                          </div>

                          <HotkeyRecorder
                            value={getHotkeyValue(profile, meta.action)}
                            onChange={(val) => updateHotkey(profile, meta.action, val)}
                            hasConflict={!!conflictMessage || rejected}
                          />

                          {conflictMessage && (
                            <p className="text-[11px] text-accent-red animate-row-in">{conflictMessage}</p>
                          )}
                          {!conflictMessage && rejected && (
                            <p className="text-[11px] text-accent-red animate-row-in">
                              {t('hotkey.notRegistered')}
                            </p>
                          )}
                        </div>
                      );
                    })}
                  </div>
                </div>
              ))}
            </div>
          );
        })}

        {profiles.length === 0 && (
          <div className="p-10 flex flex-col items-center text-center border border-dashed border-border-subtle rounded-xl">
            <div className="w-12 h-12 rounded-full bg-bg-hover/60 border border-border-subtle flex items-center justify-center text-xl">
              🎧
            </div>
            <p className="mt-4 text-sm text-gray-400 max-w-sm">{t('profiles.empty')}</p>
          </div>
        )}
      </div>

      <ProcessModal
        isOpen={isModalOpen}
        onClose={() => {
          setIsModalOpen(false);
          setProcessPickerTarget(null);
        }}
        onSelect={handleSelectProcess}
        processes={processes}
      />
    </div>
  );
};
