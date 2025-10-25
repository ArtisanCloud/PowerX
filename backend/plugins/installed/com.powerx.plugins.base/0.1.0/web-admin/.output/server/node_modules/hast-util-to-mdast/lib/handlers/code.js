/**
 * @import {State} from 'hast-util-to-mdast'
 * @import {Element} from 'hast'
 * @import {Code} from 'mdast'
 */

import {toText} from 'hast-util-to-text'
import {trimTrailingLines} from 'trim-trailing-lines'

const prefix = 'language-'

/**
 * @param {State} state
 *   State.
 * @param {Readonly<Element>} node
 *   hast element to transform.
 * @returns {Code}
 *   mdast node.
 */
export function code(state, node) {
  const children = node.children
  let index = -1
  /** @type {Array<number | string> | undefined} */
  let classList
  /** @type {string | undefined} */
  let lang

  if (node.tagName === 'pre') {
    while (++index < children.length) {
      const child = children[index]

      if (
        child.type === 'element' &&
        child.tagName === 'code' &&
        child.properties &&
        child.properties.className &&
        Array.isArray(child.properties.className)
      ) {
        classList = child.properties.className
        break
      }
    }
  }

  if (classList) {
    index = -1

    while (++index < classList.length) {
      if (String(classList[index]).slice(0, prefix.length) === prefix) {
        lang = String(classList[index]).slice(prefix.length)
        break
      }
    }
  }

  /** @type {Code} */
  const result = {
    type: 'code',
    lang: lang || null,
    meta: null,
    value: trimTrailingLines(toText(node))
  }
  state.patch(node, result)
  return result
}
