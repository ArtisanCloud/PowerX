/**
 * @import {State} from 'hast-util-to-mdast'
 * @import {Element} from 'hast'
 * @import {Break} from 'mdast'
 */

/**
 * @param {State} state
 *   State.
 * @param {Readonly<Element>} node
 *   hast element to transform.
 * @returns {Break}
 *   mdast node.
 */
export function br(state, node) {
  /** @type {Break} */
  const result = {type: 'break'}
  state.patch(node, result)
  return result
}
