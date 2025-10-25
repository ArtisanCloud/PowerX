/**
 * @import {State} from 'hast-util-to-mdast'
 * @import {Text as HastText} from 'hast'
 * @import {Text as MdastText} from 'mdast'
 */

/**
 * @param {State} state
 *   State.
 * @param {Readonly<HastText>} node
 *   hast element to transform.
 * @returns {MdastText}
 *   mdast node.
 */
export function text(state, node) {
  /** @type {MdastText} */
  const result = {type: 'text', value: node.value}
  state.patch(node, result)
  return result
}
