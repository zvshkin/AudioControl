import React, { useState, KeyboardEvent } from 'react';
import { useI18n } from '../i18n';



const MOD_ALT = 0x0001;
const MOD_CONTROL = 0x0002;
const MOD_SHIFT = 0x0004;
const MOD_WIN = 0x0008;

export interface HotkeyValue {
  /** Человекочитаемая строка для отображения, например "Ctrl + Alt + ↑" */
  combo: string;
  /** Виртуальный код клавиши Windows (VK_*) — используется backend'ом для RegisterHotKey */
  keyCode: number;
  /** Битовая маска модификаторов (MOD_ALT | MOD_CONTROL | ...) */
  modifiers: number;
}

interface HotkeyRecorderProps {
  value: HotkeyValue | null;
  onChange: (hotkey: HotkeyValue | null) => void;
  placeholder?: string;
  /** Подсвечивает поле красным — эта комбинация уже занята другим действием/профилем. */
  hasConflict?: boolean;
}



const KEY_LABELS: Record<string, string> = {
  ArrowUp: '↑',
  ArrowDown: '↓',
  ArrowLeft: '←',
  ArrowRight: '→',
  ' ': 'Space',
};

export const HotkeyRecorder: React.FC<HotkeyRecorderProps> = ({
  value,
  onChange,
  placeholder,
  hasConflict = false,
}) => {
  const { t } = useI18n();
  const [isRecording, setIsRecording] = useState(false);
  const [justAssigned, setJustAssigned] = useState(false);

  const handleKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    e.preventDefault();
    e.stopPropagation();

    
    if (['Control', 'Alt', 'Shift', 'Meta'].includes(e.key)) {
      return;
    }

    if (e.key === 'Escape') {
      (e.target as HTMLElement).blur();
      return;
    }

    let modifiers = 0;
    if (e.altKey) modifiers |= MOD_ALT;
    if (e.ctrlKey) modifiers |= MOD_CONTROL;
    if (e.shiftKey) modifiers |= MOD_SHIFT;
    if (e.metaKey) modifiers |= MOD_WIN;

    
    
    if (modifiers === 0) {
      return;
    }

    const parts: string[] = [];
    if (e.ctrlKey) parts.push('Ctrl');
    if (e.altKey) parts.push('Alt');
    if (e.shiftKey) parts.push('Shift');
    if (e.metaKey) parts.push('Win');

    const label = KEY_LABELS[e.key] ?? (e.key.length === 1 ? e.key.toUpperCase() : e.key);
    parts.push(label);

    
    
    
    const keyCode = e.keyCode;

    onChange({
      combo: parts.join(' + '),
      keyCode,
      modifiers,
    });
    setIsRecording(false);

    
    setJustAssigned(true);
    setTimeout(() => setJustAssigned(false), 180);
  };

  const keys = value?.combo ? value.combo.split(' + ') : [];

  const borderClass = hasConflict
    ? 'border-accent-red ring-1 ring-accent-red/50'
    : isRecording
    ? 'border-accent-cyan ring-1 ring-accent-cyan shadow-glow'
    : 'border-border-subtle hover:border-gray-500';

  return (
    <div
      tabIndex={0}
      onFocus={() => setIsRecording(true)}
      onBlur={() => setIsRecording(false)}
      onKeyDown={handleKeyDown}
      className={`relative flex items-center min-h-[42px] px-3 py-1.5 rounded-lg border bg-bg-main cursor-pointer outline-none transition-all duration-150 ${borderClass}`}
    >
      {isRecording && (
        <span className="absolute -top-2 left-2 px-1.5 text-[10px] font-mono bg-bg-main text-accent-cyan rounded animate-pulse-soft">
          {t('hotkey.waiting')}
        </span>
      )}

      {keys.length > 0 ? (
        <div className="flex items-center space-x-1.5 flex-wrap gap-y-1">
          {keys.map((k, i) => (
            <React.Fragment key={i}>
              <span
                className={`px-2 py-0.5 text-xs font-mono font-semibold border rounded shadow-sm transition-transform duration-150 ${
                  hasConflict
                    ? 'bg-accent-red/10 text-accent-red border-accent-red/40'
                    : 'bg-bg-hover text-accent-cyan border-accent-cyan/30'
                } ${justAssigned ? 'animate-keycap-press' : ''}`}
              >
                {k}
              </span>
              {i < keys.length - 1 && <span className="text-xs text-gray-500">+</span>}
            </React.Fragment>
          ))}
        </div>
      ) : (
        <span className="text-sm text-gray-500">
          {isRecording ? t('hotkey.recording') : placeholder ?? t('hotkey.placeholder')}
        </span>
      )}

      {value?.combo && (
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation();
            onChange(null);
          }}
          className="ml-auto text-xs text-gray-500 hover:text-accent-red transition-colors duration-150 pl-2"
          title={t('hotkey.clear')}
        >
          &#10005;
        </button>
      )}
    </div>
  );
};
