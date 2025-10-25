/**
 * @import {State} from 'hast-util-to-mdast'
 * @import {Element} from 'hast'
 * @import {Image} from 'mdast'
 */

/**
 * @param {State} state
 *   State.
 * @param {Readonly<Element>} node
 *   hast element to transform.
 * @returns {Image}
 *   mdast node.
 */
export function img(state, node) {
  const properties = node.properties || {}

  /** @type {Image} */
  const result = {
    type: 'image',
    url: state.resolve(String(properties.src || '') || null),
    title: properties.title ? String(properties.title) : null,
    alt: properties.alt ? String(properties.alt) : ''
  }
  state.patch(node, result)
  return result
}
