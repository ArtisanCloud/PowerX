/**
 * @import {State} from 'hast-util-to-mdast'
 * @import {Element} from 'hast'
 * @import {PhrasingContent, Strong} from 'mdast'
 */

/**
 * @param {State} state
 *   State.
 * @param {Readonly<Element>} node
 *   hast element to transform.
 * @returns {Strong}
 *   mdast node.
 */
export function strong(state, node) {
  // Allow potentially “invalid” nodes, they might be unknown.
  // We also support straddling later.
  const children = /** @type {Array<PhrasingContent>} */ (state.all(node))

  /** @type {Strong} */
  const result = {type: 'strong', children}
  state.patch(node, result)
  return result
}
