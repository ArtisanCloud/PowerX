/**
 * @import {State} from 'hast-util-to-mdast'
 * @import {Element} from 'hast'
 * @import {Text} from 'mdast'
 */

import {toText} from 'hast-util-to-text'

/**
 * @param {State} state
 *   State.
 * @param {Readonly<Element>} node
 *   hast element to transform.
 * @returns {Text}
 *   mdast node.
 */
export function textarea(state, node) {
  /** @type {Text} */
  const result = {type: 'text', value: toText(node)}
  state.patch(node, result)
  return result
}
