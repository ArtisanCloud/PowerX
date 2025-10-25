/**
 * @import {State} from 'hast-util-to-mdast'
 * @import {Element} from 'hast'
 * @import {Text} from 'mdast'
 */

/**
 * @param {State} state
 *   State.
 * @param {Readonly<Element>} node
 *   hast element to transform.
 * @returns {Text}
 *   mdast node.
 */
export function wbr(state, node) {
  /** @type {Text} */
  const result = {type: 'text', value: '\u200B'}
  state.patch(node, result)
  return result
}
