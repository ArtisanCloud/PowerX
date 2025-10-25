/**
 * @import {State} from 'hast-util-to-mdast'
 * @import {Element} from 'hast'
 * @import {Emphasis, PhrasingContent} from 'mdast'
 */

/**
 * @param {State} state
 *   State.
 * @param {Readonly<Element>} node
 *   hast element to transform.
 * @returns {Emphasis}
 *   mdast node.
 */
export function em(state, node) {
  // Allow potentially “invalid” nodes, they might be unknown.
  // We also support straddling later.
  const children = /** @type {Array<PhrasingContent>} */ (state.all(node))

  /** @type {Emphasis} */
  const result = {type: 'emphasis', children}
  state.patch(node, result)
  return result
}
