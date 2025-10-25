/**
 * @import {State} from 'hast-util-to-mdast'
 * @import {Element} from 'hast'
 * @import {Delete, PhrasingContent} from 'mdast'
 */

/**
 * @param {State} state
 *   State.
 * @param {Readonly<Element>} node
 *   hast element to transform.
 * @returns {Delete}
 *   mdast node.
 */
export function del(state, node) {
  // Allow potentially “invalid” nodes, they might be unknown.
  // We also support straddling later.
  const children = /** @type {Array<PhrasingContent>} */ (state.all(node))
  /** @type {Delete} */
  const result = {type: 'delete', children}
  state.patch(node, result)
  return result
}
