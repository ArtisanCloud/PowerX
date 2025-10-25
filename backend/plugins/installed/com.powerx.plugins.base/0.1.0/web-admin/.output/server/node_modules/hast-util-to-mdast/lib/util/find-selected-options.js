/**
 * @import {Element, Properties} from 'hast'
 */

/**
 * @typedef {[string, Value]} Option
 *   Option, where the item at `0` is the label, the item at `1` the value.
 *
 * @typedef {Array<Option>} Options
 *   List of options.
 *
 * @typedef {string | undefined} Value
 *   `value` field of option.
 */

import {toText} from 'hast-util-to-text'

/**
 * @param {Readonly<Element>} node
 *   hast element to inspect.
 * @param {Properties | undefined} [explicitProperties]
 *   Properties to use, normally taken from `node`, but can be changed.
 * @returns {Options}
 *   Options.
 */
export function findSelectedOptions(node, explicitProperties) {
  /** @type {Array<Element>} */
  const selectedOptions = []
  /** @type {Options} */
  const values = []
  const properties = explicitProperties || node.properties || {}
  const options = findOptions(node)
  const size =
    Math.min(Number.parseInt(String(properties.size), 10), 0) ||
    (properties.multiple ? 4 : 1)
  let index = -1

  while (++index < options.length) {
    const option = options[index]

    if (option && option.properties && option.properties.selected) {
      selectedOptions.push(option)
    }
  }

  const list = selectedOptions.length > 0 ? selectedOptions : options
  const max = Math.min(list.length, size)
  index = -1

  while (++index < max) {
    const option = list[index]
    const properties = option.properties || {}
    const content = toText(option)
    const label = content || String(properties.label || '')
    const value = String(properties.value || '') || content
    values.push([value, label === value ? undefined : label])
  }

  return values
}

/**
 * @param {Element} node
 *   Parent to find in.
 * @returns {Array<Element>}
 *   Option elements.
 */
function findOptions(node) {
  /** @type {Array<Element>} */
  const results = []
  let index = -1

  while (++index < node.children.length) {
    const child = node.children[index]

    if ('children' in child && Array.isArray(child.children)) {
      results.push(...findOptions(child))
    }

    if (
      child.type === 'element' &&
      child.tagName === 'option' &&
      (!child.properties || !child.properties.disabled)
    ) {
      results.push(child)
    }
  }

  return results
}
