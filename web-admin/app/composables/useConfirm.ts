import { useOverlay } from '#imports'
import { CommonConfirmModal } from '#components'

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
  // 不使用 Lazy 版本，避免点击时再去拉取组件 chunk 导致 pending/卡死
  const modal = overlay.create(CommonConfirmModal)

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
