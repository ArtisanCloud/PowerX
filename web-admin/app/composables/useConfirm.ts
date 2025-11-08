import { useOverlay } from '#imports'
import { LazyCommonConfirmModal } from '#components'

export type ConfirmOptions = {
  title?: string
  description?: string
  message?: string
  confirmLabel?: string
  cancelLabel?: string
  confirmColor?: 'primary' | 'neutral' | 'red' | 'green' | 'blue' | 'yellow'
  tone?: 'danger' | 'warning' | 'success' | 'info'
  showIcon?: boolean
}

export const useConfirm = () => {
  const overlay = useOverlay()
  const modal = overlay.create(LazyCommonConfirmModal)

  const confirm = async (opts: ConfirmOptions = {}) => {
    const instance = modal.open({
      title: opts.title,
      description: opts.description,
      message: opts.message,
      confirmLabel: opts.confirmLabel,
      cancelLabel: opts.cancelLabel,
      confirmColor: opts.confirmColor,
      tone: opts.tone,
      showIcon: opts.showIcon,
    })
    const result = await instance.result
    return Boolean(result)
  }

  return { confirm }
}
