/**
 * @import {State} from 'hast-util-to-mdast'
 * @import {Element} from 'hast'
 * @import {ThematicBreak} from 'mdast'
 */

/**
 * @param {State} state
 *   State.
 * @param {Readonly<Element>} node
 *   hast element to transform.
 * @returns {ThematicBreak}
 *   mdast node.
 */
export function hr(state, node) {
  /** @type {ThematicBreak} */
  const result = {type: 'thematicBreak'}
  state.patch(node, result)
  return result
}
