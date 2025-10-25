import { useOverlay } from '#imports'
import { LazyCommonPromptModal } from '#components'

export type PromptOptions = {
  title?: string
  description?: string
  placeholder?: string
  defaultValue?: string
  confirmLabel?: string
  cancelLabel?: string
}

export const usePrompt = () => {
  const overlay = useOverlay()
  const modal = overlay.create(LazyCommonPromptModal)

  const prompt = async (opts: PromptOptions = {}) => {
    const instance = modal.open({
      title: opts.title,
      description: opts.description,
      placeholder: opts.placeholder,
      defaultValue: opts.defaultValue,
      confirmLabel: opts.confirmLabel,
      cancelLabel: opts.cancelLabel,
    })
    const result = await instance.result
    // null 表示取消
    return (result ?? null) as string | null
  }

  return { prompt }
}

