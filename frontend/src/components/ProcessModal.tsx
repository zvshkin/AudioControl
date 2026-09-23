import React, { useEffect, useMemo, useRef, useState } from 'react';
import { process } from '../../wailsjs/go/models';
import { useI18n } from '../i18n';

interface ProcessModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSelect: (process: process.ProcessInfo) => void;
  processes: process.ProcessInfo[];
}

export const ProcessModal: React.FC<ProcessModalProps> = ({
  isOpen,
  onClose,
  onSelect,
  processes,
}) => {
  const { t } = useI18n();
  const [search, setSearch] = useState('');
  const [debouncedSearch, setDebouncedSearch] = useState('');
  const [activeIndex, setActiveIndex] = useState(0);
  const listRef = useRef<HTMLDivElement>(null);

  
  
  useEffect(() => {
    const t = setTimeout(() => setDebouncedSearch(search), 120);
    return () => clearTimeout(t);
  }, [search]);

  useEffect(() => {
    if (isOpen) {
      setSearch('');
      setDebouncedSearch('');
      setActiveIndex(0);
    }
  }, [isOpen]);

  const filtered = useMemo(() => {
    const q = debouncedSearch.toLowerCase();
    if (!q) return processes;
    return processes.filter(
      (p) =>
        p.display_name.toLowerCase().includes(q) ||
        p.process_name.toLowerCase().includes(q) ||
        p.pid.toString().includes(q)
    );
  }, [processes, debouncedSearch]);

  useEffect(() => {
    setActiveIndex(0);
  }, [debouncedSearch]);

  useEffect(() => {
    const el = listRef.current?.querySelector(`[data-index="${activeIndex}"]`);
    el?.scrollIntoView({ block: 'nearest' });
  }, [activeIndex]);

  if (!isOpen) return null;

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      setActiveIndex((i) => Math.min(i + 1, filtered.length - 1));
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      setActiveIndex((i) => Math.max(i - 1, 0));
    } else if (e.key === 'Enter') {
      e.preventDefault();
      const chosen = filtered[activeIndex];
      if (chosen) {
        onSelect(chosen);
        onClose();
      }
    } else if (e.key === 'Escape') {
      e.preventDefault();
      onClose();
    }
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4 animate-overlay-in"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div className="w-full max-w-md bg-bg-main border border-border-subtle rounded-xl shadow-2xl flex flex-col max-h-[80vh] overflow-hidden animate-modal-in">
        {}
        <div className="p-4 border-b border-border-subtle flex items-center justify-between">
          <h3 className="text-sm font-semibold text-white uppercase tracking-wider font-mono">
            {t('modal.selectProcess')}
          </h3>
          <button
            onClick={onClose}
            className="text-gray-400 hover:text-white text-lg leading-none transition-colors duration-150"
          >
            &#10005;
          </button>
        </div>

        {}
        <div className="p-3 border-b border-border-subtle bg-bg-hover/30">
          <input
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder={t('modal.search')}
            className="w-full px-3 py-2 text-sm bg-bg-main border border-border-subtle rounded-lg text-white placeholder-gray-500 focus:outline-none focus:border-accent-cyan transition-colors duration-150"
            autoFocus
          />
          <p className="text-[10px] text-gray-500 mt-1.5 font-mono">
            {t('modal.navHint')}
          </p>
        </div>

        {}
        <div ref={listRef} className="flex-1 overflow-y-auto p-2 space-y-1">
          {filtered.length > 0 ? (
            filtered.map((proc, index) => (
              <div
                key={proc.pid}
                data-index={index}
                onMouseEnter={() => setActiveIndex(index)}
                onClick={() => {
                  onSelect(proc);
                  onClose();
                }}
                className={`flex items-center p-2.5 rounded-lg cursor-pointer transition-colors duration-150 group ${
                  index === activeIndex
                    ? 'bg-accent-cyan/15 ring-1 ring-accent-cyan/40'
                    : 'hover:bg-bg-hover'
                }`}
              >
                {proc.icon_base64 ? (
                  <img
                    src={proc.icon_base64}
                    alt=""
                    className="w-6 h-6 rounded mr-3 flex-shrink-0"
                  />
                ) : (
                  <div className="w-6 h-6 rounded mr-3 flex-shrink-0 bg-bg-hover border border-border-subtle" />
                )}
                <div className="flex flex-col truncate pr-2 flex-1">
                  <span
                    className={`text-sm font-medium truncate transition-colors duration-150 ${
                      index === activeIndex ? 'text-accent-cyan' : 'text-gray-200 group-hover:text-accent-cyan'
                    }`}
                  >
                    {proc.display_name || proc.process_name}
                  </span>
                  <span className="text-xs text-gray-500 truncate">{proc.process_name}</span>
                </div>
                <span className="text-xs font-mono text-gray-400 bg-bg-main px-2 py-0.5 rounded border border-border-subtle flex-shrink-0">
                  PID: {proc.pid}
                </span>
              </div>
            ))
          ) : (
            <div className="p-6 text-center text-xs text-gray-500">
              {processes.length === 0 ? t('modal.noProcesses') : t('modal.nothingFound')}
            </div>
          )}
        </div>
      </div>
    </div>
  );
};
