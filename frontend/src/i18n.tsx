import React, { createContext, useCallback, useContext, useMemo, useState } from 'react';

export type Lang = 'ru' | 'en';



const DICT = {
  ru: {
    'nav.profiles': 'Профили',
    'nav.settings': 'Настройки',

    'titlebar.minimize': 'Свернуть',
    'titlebar.hide': 'Свернуть в трей',

    'profiles.title': 'Профили громкости',
    'profiles.subtitle': 'Изменения сохраняются автоматически',
    'profiles.add': '+ Добавить профиль',
    'profiles.empty': 'Профилей пока нет — нажмите «Добавить профиль», чтобы начать.',
    'profiles.process': 'Процесс',
    'profiles.processPlaceholder': 'Выберите процесс...',
    'profiles.active': 'Активен',
    'profiles.disabled': 'Отключен',
    'profiles.delete': 'Удалить профиль',
    'profiles.deleteConfirm': 'Точно удалить?',
    'profiles.step': 'Шаг',
    'profiles.stepHint': 'На сколько процентов менять громкость за одно нажатие',
    'profiles.mediaTarget': 'Медиа-цель',
    'profiles.mediaTargetAuto': 'Автоматически (первая найденная)',
    'profiles.mediaTargetFound': 'найдено сессий: {count}',
    'profiles.mediaTargetHint':
      'Если медиа-клавиши не срабатывают, выберите нужную сессию здесь вручную: идентификатор media-сессии не всегда совпадает с именем процесса.',
    'profiles.noSession': 'Нет аудиосессии',
    'profiles.noSessionHint': 'Процесс закрыт или сейчас не воспроизводит звук',
    'profiles.live': 'Звучит',
    'profiles.unmute': 'Включить звук',
    'profiles.mute': 'Выключить звук',
    'profiles.duplicate': 'Дублировать профиль',
    'profiles.moveUp': 'Выше',
    'profiles.moveDown': 'Ниже',
    'settings.startMinimized': 'Запускать свёрнутым в трей',
    'settings.startMinimizedHint': 'Окно не будет открываться при запуске — особенно полезно вместе с автозапуском.',
    'toast.profileDuplicated': 'Профиль скопирован',
    'toast.startMinimizedOn': 'Запуск свёрнутым включён',
    'toast.startMinimizedOff': 'Запуск свёрнутым выключен',
    'toast.errVolume': 'Не удалось изменить громкость',
    'profiles.test': 'Проверить',
    'profiles.testHint': 'Выполнить действие сейчас, без нажатия хоткея',

    'group.volume': 'Громкость',
    'group.media': 'Медиаклавиши',

    'action.volume_up': 'Громкость +',
    'action.volume_down': 'Громкость −',
    'action.toggle_mute': 'Mute',
    'action.play_pause': 'Пауза / Плей',
    'action.next_track': 'Следующий трек',
    'action.prev_track': 'Предыдущий трек',

    'hotkey.placeholder': 'Нажмите комбинацию...',
    'hotkey.recording': 'Нажмите комбинацию клавиш...',
    'hotkey.waiting': 'ожидание клавиш...',
    'hotkey.clear': 'Очистить',
    'hotkey.conflict': 'Уже используется: «{profile}» → {action}',
    'hotkey.notRegistered': 'Windows отклонила эту комбинацию — вероятно, она занята другой программой',

    'hotkeys.globalOff':
      'Глобальные хоткеи сейчас выключены — ни один хоткей ниже не сработает, пока вы их не включите.',

    'modal.selectProcess': 'Выберите процесс',
    'modal.search': 'Поиск по имени или PID...',
    'modal.navHint': '↑↓ навигация · Enter выбрать · Esc закрыть',
    'modal.noProcesses': 'Активные процессы не найдены',
    'modal.nothingFound': 'Ничего не найдено по запросу',

    'settings.title': 'Настройки',
    'settings.subtitle': 'Интерфейс, хоткеи, автозапуск и обновления',
    'settings.hotkeys': 'Хоткеи',
    'settings.globalHotkeys': 'Глобальные хоткеи',
    'settings.globalHotkeysHint':
      'При выключении все комбинации реально снимаются с регистрации в системе.',
    'settings.diagnostics': 'Состояние хоткеев',
    'settings.diagnosticsRefresh': 'Обновить',
    'settings.diagnosticsEmpty': 'Зарегистрированных хоткеев нет.',
    'settings.registered': 'работает',
    'settings.notRegistered': 'отклонён',
    'settings.interface': 'Интерфейс',
    'settings.language': 'Язык',
    'settings.startup': 'Запуск',
    'settings.autostart': 'Запускать вместе с Windows',
    'settings.autostartHint': 'Приложение будет стартовать свёрнутым в трей при входе в систему.',
    'settings.updates': 'Обновления',
    'settings.checkUpdates': 'Проверить обновления',
    'settings.checking': 'Проверка...',
    'settings.currentVersion': 'Текущая версия:',
    'settings.upToDate': 'У вас установлена последняя версия!',
    'settings.newVersion': 'Доступна новая версия ({version}).',
    'settings.download': 'Скачать на GitHub',
    'settings.support': 'Поддержка',
    'settings.supportLink': 'Написать в поддержку',
    'settings.about': 'О программе',
    'settings.aboutText':
      'AudioControl Central — лёгкая утилита для управления громкостью отдельных процессов и медиаклавишами конкретных приложений с помощью глобальных хоткеев.',

    'toast.saved': 'Сохранено',
    'toast.profileCreated': 'Профиль «{name}» создан',
    'toast.profileDeleted': 'Профиль удалён',
    'toast.profileEnabled': '«{name}» включён',
    'toast.profileDisabled': '«{name}» отключён',
    'toast.processChanged': 'Процесс изменён на «{name}»',
    'toast.hotkeySet': '«{action}» → {combo}',
    'toast.hotkeyCleared': '«{action}» сброшен',
    'toast.stepChanged': 'Шаг громкости: {step}%',
    'toast.mediaTargetChanged': 'Медиа-цель обновлена',
    'toast.hotkeysOn': 'Хоткеи включены',
    'toast.hotkeysOff': 'Хоткеи выключены',
    'toast.langChanged': 'Язык интерфейса изменён',
    'toast.autostartOn': 'Автозапуск включён',
    'toast.autostartOff': 'Автозапуск выключен',
    'toast.mediaUnavailable': 'Media-цель недоступна',
    'toast.mediaUnavailableText':
      '«{process}» сейчас не публикует media-сессию — {action} не выполнено.',
    'toast.actionFailed': 'Действие не выполнено',
    'toast.hotkeyRegIssue': 'Часть хоткеев не зарегистрирована',
    'toast.testOk': 'Готово: {result}',
    'toast.errLoad': 'Не удалось загрузить конфигурацию',
    'toast.errSave': 'Не удалось сохранить профиль',
    'toast.errDelete': 'Не удалось удалить профиль',
    'toast.errProcesses': 'Не удалось получить список процессов',
    'toast.errHotkeysToggle': 'Не удалось переключить хоткеи',
    'toast.errLang': 'Не удалось сменить язык',
    'toast.errAutostart': 'Не удалось изменить автозапуск',
  },

  en: {
    'nav.profiles': 'Profiles',
    'nav.settings': 'Settings',

    'titlebar.minimize': 'Minimize',
    'titlebar.hide': 'Hide to tray',

    'profiles.title': 'Volume profiles',
    'profiles.subtitle': 'Changes are saved automatically',
    'profiles.add': '+ Add profile',
    'profiles.empty': 'No profiles yet — click "Add profile" to get started.',
    'profiles.process': 'Process',
    'profiles.processPlaceholder': 'Select a process...',
    'profiles.active': 'Active',
    'profiles.disabled': 'Disabled',
    'profiles.delete': 'Delete profile',
    'profiles.deleteConfirm': 'Really delete?',
    'profiles.step': 'Step',
    'profiles.stepHint': 'How many percent to change the volume per key press',
    'profiles.mediaTarget': 'Media target',
    'profiles.mediaTargetAuto': 'Automatic (first match)',
    'profiles.mediaTargetFound': 'sessions found: {count}',
    'profiles.mediaTargetHint':
      'If media keys do not work, pick the right session here manually: a media session id does not always match the process name.',
    'profiles.noSession': 'No audio session',
    'profiles.noSessionHint': 'The process is closed or not playing anything right now',
    'profiles.live': 'Playing',
    'profiles.unmute': 'Unmute',
    'profiles.mute': 'Mute',
    'profiles.duplicate': 'Duplicate profile',
    'profiles.moveUp': 'Move up',
    'profiles.moveDown': 'Move down',
    'settings.startMinimized': 'Start minimized to tray',
    'settings.startMinimizedHint': 'The window will not open on launch — especially useful together with autostart.',
    'toast.profileDuplicated': 'Profile duplicated',
    'toast.startMinimizedOn': 'Start minimized enabled',
    'toast.startMinimizedOff': 'Start minimized disabled',
    'toast.errVolume': 'Failed to change the volume',
    'profiles.test': 'Test',
    'profiles.testHint': 'Run the action right now, without pressing the hotkey',

    'group.volume': 'Volume',
    'group.media': 'Media keys',

    'action.volume_up': 'Volume +',
    'action.volume_down': 'Volume −',
    'action.toggle_mute': 'Mute',
    'action.play_pause': 'Play / Pause',
    'action.next_track': 'Next track',
    'action.prev_track': 'Previous track',

    'hotkey.placeholder': 'Press a combination...',
    'hotkey.recording': 'Press a key combination...',
    'hotkey.waiting': 'waiting for keys...',
    'hotkey.clear': 'Clear',
    'hotkey.conflict': 'Already used by "{profile}" → {action}',
    'hotkey.notRegistered': 'Windows rejected this combination — it is probably taken by another app',

    'hotkeys.globalOff':
      'Global hotkeys are currently disabled — none of the hotkeys below will work until you enable them.',

    'modal.selectProcess': 'Select a process',
    'modal.search': 'Search by name or PID...',
    'modal.navHint': '↑↓ navigate · Enter select · Esc close',
    'modal.noProcesses': 'No active processes found',
    'modal.nothingFound': 'Nothing matches your search',

    'settings.title': 'Settings',
    'settings.subtitle': 'Interface, hotkeys, startup and updates',
    'settings.hotkeys': 'Hotkeys',
    'settings.globalHotkeys': 'Global hotkeys',
    'settings.globalHotkeysHint':
      'When disabled, all combinations are actually unregistered from the system.',
    'settings.diagnostics': 'Hotkey status',
    'settings.diagnosticsRefresh': 'Refresh',
    'settings.diagnosticsEmpty': 'No registered hotkeys.',
    'settings.registered': 'working',
    'settings.notRegistered': 'rejected',
    'settings.interface': 'Interface',
    'settings.language': 'Language',
    'settings.startup': 'Startup',
    'settings.autostart': 'Launch with Windows',
    'settings.autostartHint': 'The app will start minimized to tray when you sign in.',
    'settings.updates': 'Updates',
    'settings.checkUpdates': 'Check for updates',
    'settings.checking': 'Checking...',
    'settings.currentVersion': 'Current version:',
    'settings.upToDate': 'You are running the latest version!',
    'settings.newVersion': 'A new version is available ({version}).',
    'settings.download': 'Download on GitHub',
    'settings.support': 'Support',
    'settings.supportLink': 'Contact support',
    'settings.about': 'About',
    'settings.aboutText':
      'AudioControl Central is a lightweight utility for controlling per-process volume and per-app media keys via global hotkeys.',

    'toast.saved': 'Saved',
    'toast.profileCreated': 'Profile "{name}" created',
    'toast.profileDeleted': 'Profile deleted',
    'toast.profileEnabled': '"{name}" enabled',
    'toast.profileDisabled': '"{name}" disabled',
    'toast.processChanged': 'Process changed to "{name}"',
    'toast.hotkeySet': '"{action}" → {combo}',
    'toast.hotkeyCleared': '"{action}" cleared',
    'toast.stepChanged': 'Volume step: {step}%',
    'toast.mediaTargetChanged': 'Media target updated',
    'toast.hotkeysOn': 'Hotkeys enabled',
    'toast.hotkeysOff': 'Hotkeys disabled',
    'toast.langChanged': 'Interface language changed',
    'toast.autostartOn': 'Autostart enabled',
    'toast.autostartOff': 'Autostart disabled',
    'toast.mediaUnavailable': 'Media target unavailable',
    'toast.mediaUnavailableText':
      '"{process}" is not publishing a media session right now — {action} was not performed.',
    'toast.actionFailed': 'Action failed',
    'toast.hotkeyRegIssue': 'Some hotkeys were not registered',
    'toast.testOk': 'Done: {result}',
    'toast.errLoad': 'Failed to load configuration',
    'toast.errSave': 'Failed to save profile',
    'toast.errDelete': 'Failed to delete profile',
    'toast.errProcesses': 'Failed to get the process list',
    'toast.errHotkeysToggle': 'Failed to toggle hotkeys',
    'toast.errLang': 'Failed to change language',
    'toast.errAutostart': 'Failed to change autostart',
  },
} as const;

export type TranslationKey = keyof (typeof DICT)['ru'];

type AssertTrue<T extends true> = T;






export type RuEnKeysMatch = AssertTrue<
  TranslationKey extends keyof (typeof DICT)['en'] ? true : false
> &
  AssertTrue<keyof (typeof DICT)['en'] extends TranslationKey ? true : false>;

export type TranslateFn = (key: TranslationKey, vars?: Record<string, string | number>) => string;




export const ACTION_TYPES = [
  'volume_up',
  'volume_down',
  'toggle_mute',
  'play_pause',
  'next_track',
  'prev_track',
] as const;

export type ActionType = (typeof ACTION_TYPES)[number];

export function isActionType(value: string): value is ActionType {
  return (ACTION_TYPES as readonly string[]).includes(value);
}






export function actionLabel(t: TranslateFn, action: string): string {
  return isActionType(action) ? t(`action.${action}`) : action;
}

interface I18nContextValue {
  lang: Lang;
  setLang: (lang: Lang) => void;
  t: TranslateFn;
}

const I18nContext = createContext<I18nContextValue | null>(null);

export const I18nProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [lang, setLang] = useState<Lang>('ru');

  const t = useCallback<TranslateFn>(
    (key, vars) => {
      
      
      const table = DICT[lang] as Record<string, string>;
      let str = table[key] ?? (DICT.ru as Record<string, string>)[key] ?? key;

      if (vars) {
        for (const [k, v] of Object.entries(vars)) {
          str = str.replace(new RegExp(`\\{${k}\\}`, 'g'), String(v));
        }
      }
      return str;
    },
    [lang]
  );

  const value = useMemo(() => ({ lang, setLang, t }), [lang, t]);

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
};

export function useI18n(): I18nContextValue {
  const ctx = useContext(I18nContext);
  if (!ctx) {
    throw new Error('useI18n must be used within an I18nProvider');
  }
  return ctx;
}
