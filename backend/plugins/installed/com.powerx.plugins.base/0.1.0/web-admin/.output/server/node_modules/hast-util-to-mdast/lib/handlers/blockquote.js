/**
 * @import {State} from 'hast-util-to-mdast'
 * @import {Element} from 'hast'
 * @import {Blockquote} from 'mdast'
 */

/**
 * @param {State} state
 *   State.
 * @param {Readonly<Element>} node
 *   hast element to transform.
 * @returns {Blockquote}
 *   mdast node.
 */
export function blockquote(state, node) {
  /** @type {Blockquote} */
  const result = {type: 'blockquote', children: state.toFlow(state.all(node))}
  state.patch(node, result)
  return result
}
