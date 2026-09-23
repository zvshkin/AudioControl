import React, { createContext, useCallback, useContext, useRef, useState } from 'react';

type ToastKind = 'success' | 'error' | 'warning' | 'info';

interface ToastItem {
  id: number;
  kind: ToastKind;
  title: string;
  description?: string;
  /** Поднимается при начале анимации исчезновения (см. dismiss). */
  _leaving?: boolean;
}

interface ToastContextValue {
  show: (kind: ToastKind, title: string, description?: string) => void;
}

const ToastContext = createContext<ToastContextValue | null>(null);

const KIND_STYLES: Record<ToastKind, { border: string; icon: string; iconColor: string }> = {
  success: { border: 'border-l-accent-cyan', icon: '✓', iconColor: 'text-accent-cyan' },
  error: { border: 'border-l-accent-red', icon: '✕', iconColor: 'text-accent-red' },
  warning: { border: 'border-l-yellow-500', icon: '!', iconColor: 'text-yellow-500' },
  info: { border: 'border-l-gray-400', icon: 'i', iconColor: 'text-gray-300' },
};

let nextId = 1;

export const ToastProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [toasts, setToasts] = useState<ToastItem[]>([]);
  const timers = useRef<Map<number, ReturnType<typeof setTimeout>>>(new Map());

  const dismiss = useCallback((id: number) => {
    setToasts((prev) => prev.map((t) => (t.id === id ? { ...t, _leaving: true } : t)));
    
    setTimeout(() => {
      setToasts((prev) => prev.filter((t) => t.id !== id));
    }, 180);

    const timer = timers.current.get(id);
    if (timer) {
      clearTimeout(timer);
      timers.current.delete(id);
    }
  }, []);

  const show = useCallback(
    (kind: ToastKind, title: string, description?: string) => {
      const id = nextId++;
      setToasts((prev) => [...prev, { id, kind, title, description }]);

      const timer = setTimeout(() => dismiss(id), 4000);
      timers.current.set(id, timer);
    },
    [dismiss]
  );

  return (
    <ToastContext.Provider value={{ show }}>
      {children}

      <div className="fixed bottom-4 right-4 z-[100] flex flex-col items-end space-y-2 pointer-events-none">
        {toasts.map((t) => {
          const style = KIND_STYLES[t.kind];
          const leaving = t._leaving;
          return (
            <div
              key={t.id}
              onClick={() => dismiss(t.id)}
              className={`pointer-events-auto max-w-sm w-80 bg-bg-hover border border-border-subtle ${style.border} border-l-4 rounded-lg shadow-xl px-3 py-2.5 cursor-pointer transition-all duration-200 ease-out ${
                leaving ? 'opacity-0 translate-y-1' : 'opacity-100 translate-y-0 animate-toast-in'
              }`}
            >
              <div className="flex items-start space-x-2.5">
                <span className={`mt-0.5 text-xs font-bold ${style.iconColor}`}>{style.icon}</span>
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-medium text-gray-100 leading-snug">{t.title}</p>
                  {t.description && (
                    <p className="text-xs text-gray-400 mt-0.5 leading-snug">{t.description}</p>
                  )}
                </div>
              </div>
            </div>
          );
        })}
      </div>
    </ToastContext.Provider>
  );
};

export function useToast(): ToastContextValue {
  const ctx = useContext(ToastContext);
  if (!ctx) {
    throw new Error('useToast must be used within a ToastProvider');
  }
  return ctx;
}
