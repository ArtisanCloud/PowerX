/**
 * @import {State} from 'hast-util-to-mdast'
 * @import {Element} from 'hast'
 * @import {Image, Link, Text} from 'mdast'
 * @import {Options} from '../util/find-selected-options.js'
 */

import {findSelectedOptions} from '../util/find-selected-options.js'

const defaultChecked = '[x]'
const defaultUnchecked = '[ ]'

/**
 * @param {State} state
 *   State.
 * @param {Readonly<Element>} node
 *   hast element to transform.
 * @returns {Array<Link | Text> | Image | Text | undefined}
 *   mdast node.
 */
// eslint-disable-next-line complexity
export function input(state, node) {
  const properties = node.properties || {}
  const value = String(properties.value || properties.placeholder || '')

  if (
    properties.disabled ||
    properties.type === 'hidden' ||
    properties.type === 'file'
  ) {
    return
  }

  if (properties.type === 'checkbox' || properties.type === 'radio') {
    /** @type {Text} */
    const result = {
      type: 'text',
      value: properties.checked
        ? state.options.checked || defaultChecked
        : state.options.unchecked || defaultUnchecked
    }
    state.patch(node, result)
    return result
  }

  if (properties.type === 'image') {
    const alt = properties.alt || value

    if (alt) {
      /** @type {Image} */
      const result = {
        type: 'image',
        url: state.resolve(String(properties.src || '') || null),
        title: String(properties.title || '') || null,
        alt: String(alt)
      }
      state.patch(node, result)
      return result
    }

    return
  }

  /** @type {Options} */
  let values = []

  if (value) {
    values = [[value, undefined]]
  } else if (
    // `list` is not supported on these types:
    properties.type !== 'button' &&
    properties.type !== 'file' &&
    properties.type !== 'password' &&
    properties.type !== 'reset' &&
    properties.type !== 'submit' &&
    properties.list
  ) {
    const list = String(properties.list)
    const datalist = state.elementById.get(list)

    if (datalist && datalist.tagName === 'datalist') {
      values = findSelectedOptions(datalist, properties)
    }
  }

  if (values.length === 0) {
    return
  }

  // Hide password value.
  if (properties.type === 'password') {
    // Passwords don’t support `list`.
    values[0] = ['•'.repeat(values[0][0].length), undefined]
  }

  if (properties.type === 'email' || properties.type === 'url') {
    /** @type {Array<Link | Text>} */
    const results = []
    let index = -1

    while (++index < values.length) {
      const value = state.resolve(values[index][0])
      /** @type {Link} */
      const result = {
        type: 'link',
        title: null,
        url: properties.type === 'email' ? 'mailto:' + value : value,
        children: [{type: 'text', value: values[index][1] || value}]
      }

      results.push(result)

      if (index !== values.length - 1) {
        results.push({type: 'text', value: ', '})
      }
    }

    return results
  }

  /** @type {Array<string>} */
  const texts = []
  let index = -1

  while (++index < values.length) {
    texts.push(
      values[index][1]
        ? values[index][1] + ' (' + values[index][0] + ')'
        : values[index][0]
    )
  }

  /** @type {Text} */
  const result = {type: 'text', value: texts.join(', ')}
  state.patch(node, result)
  return result
}
