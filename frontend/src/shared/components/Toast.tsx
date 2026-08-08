import { CheckCircle, XCircle, AlertCircle, Info, X } from 'lucide-react'
import { create } from 'zustand'
import { useNotificationStore } from '../../store/notificationStore'

type ToastType = 'success' | 'error' | 'warning' | 'info'

interface Toast {
  id: string
  type: ToastType
  message: string
  duration?: number
}

interface ToastStore {
  toasts: Toast[]
  addToast: (toast: Omit<Toast, 'id'>) => void
  removeToast: (id: string) => void
}

export const useToastStore = create<ToastStore>((set) => ({
  toasts: [],
  addToast: (toast) => {
    const id = Math.random().toString(36).substring(7)
    set((state) => ({
      toasts: [...state.toasts, { ...toast, id }],
    }))

    // 自动移除
    const duration = toast.duration ?? 3000
    if (duration > 0) {
      setTimeout(() => {
        set((state) => ({
          toasts: state.toasts.filter((t) => t.id !== id),
        }))
      }, duration)
    }
  },
  removeToast: (id) =>
    set((state) => ({
      toasts: state.toasts.filter((t) => t.id !== id),
    })),
}))

// Toast 工具函数
function recordErrorNotification(message: string) {
  useNotificationStore.getState().addNotification({
    type: 'error',
    title: '操作异常',
    message,
  })
}

export const toast = {
  success: (message: string, duration?: number) =>
    useToastStore.getState().addToast({ type: 'success', message, duration }),
  error: (message: string, duration?: number) => {
    useToastStore.getState().addToast({ type: 'error', message, duration })
    recordErrorNotification(message)
  },
  warning: (message: string, duration?: number) =>
    useToastStore.getState().addToast({ type: 'warning', message, duration }),
  info: (message: string, duration?: number) =>
    useToastStore.getState().addToast({ type: 'info', message, duration }),
}

const icons = {
  success: CheckCircle,
  error: XCircle,
  warning: AlertCircle,
  info: Info,
}

const barStyles = {
  success: 'bg-[var(--success)]',
  error: 'bg-[var(--danger)]',
  warning: 'bg-[var(--warning)]',
  info: 'bg-[var(--info)]',
}

const iconStyles = {
  success: 'text-[var(--success)]',
  error: 'text-[var(--danger)]',
  warning: 'text-[var(--warning)]',
  info: 'text-[var(--info)]',
}

function ToastItem({ toast: t }: { toast: Toast }) {
  const removeToast = useToastStore((state) => state.removeToast)
  const Icon = icons[t.type]

  return (
    <div
      className="relative flex items-start gap-3 w-[360px] max-w-full pl-4 pr-3 py-3 rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--bg-overlay)] shadow-[var(--shadow-lg)] animate-slide-in-right overflow-hidden"
    >
      <span
        aria-hidden="true"
        className={`absolute left-0 top-0 bottom-0 w-[3px] ${barStyles[t.type]}`}
      />
      <Icon className={`w-5 h-5 flex-shrink-0 mt-[3px] ${iconStyles[t.type]}`} aria-hidden="true" />
      <p className="flex-1 text-[13px] font-medium text-[var(--text-primary)] leading-5">{t.message}</p>
      <button
        onClick={() => removeToast(t.id)}
        className="p-1 rounded-[var(--radius-sm)] text-[var(--text-muted)] hover:bg-[var(--bg-hover)] hover:text-[var(--text-primary)] transition-colors"
      >
        <X className="w-4 h-4" />
      </button>
    </div>
  )
}

export function ToastContainer() {
  const toasts = useToastStore((state) => state.toasts)

  return (
    <div className="fixed top-3 right-3 z-[var(--z-toast)] flex flex-col gap-2 max-w-md">
      {toasts.map((t) => (
        <ToastItem key={t.id} toast={t} />
      ))}
    </div>
  )
}
