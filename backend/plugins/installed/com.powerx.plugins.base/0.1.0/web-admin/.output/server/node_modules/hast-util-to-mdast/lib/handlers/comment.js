/**
 * @import {State} from 'hast-util-to-mdast'
 * @import {Comment} from 'hast'
 * @import {Html} from 'mdast'
 */

/**
 * @param {State} state
 *   State.
 * @param {Readonly<Comment>} node
 *   hast element to transform.
 * @returns {Html}
 *   mdast node.
 */
export function comment(state, node) {
  /** @type {Html} */
  const result = {
    type: 'html',
    value: '<!--' + node.value + '-->'
  }
  state.patch(node, result)
  return result
}
