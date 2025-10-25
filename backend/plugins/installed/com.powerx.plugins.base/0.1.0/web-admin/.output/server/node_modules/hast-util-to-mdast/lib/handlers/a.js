/**
 * @import {State} from 'hast-util-to-mdast'
 * @import {Element} from 'hast'
 * @import {Link, PhrasingContent} from 'mdast'
 */

/**
 * @param {State} state
 *   State.
 * @param {Readonly<Element>} node
 *   hast element to transform.
 * @returns {Link}
 *   mdast node.
 */
export function a(state, node) {
  const properties = node.properties || {}
  // Allow potentially “invalid” nodes, they might be unknown.
  // We also support straddling later.
  const children = /** @type {Array<PhrasingContent>} */ (state.all(node))

  /** @type {Link} */
  const result = {
    type: 'link',
    url: state.resolve(String(properties.href || '') || null),
    title: properties.title ? String(properties.title) : null,
    children
  }
  state.patch(node, result)
  return result
}
