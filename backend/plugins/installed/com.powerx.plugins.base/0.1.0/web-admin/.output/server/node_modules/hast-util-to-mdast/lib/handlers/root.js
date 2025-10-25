/**
 * @import {State} from 'hast-util-to-mdast'
 * @import {Root as HastRoot} from 'hast'
 * @import {Root as MdastRoot} from 'mdast'
 */

import {wrap, wrapNeeded} from '../util/wrap.js'

/**
 * @param {State} state
 *   State.
 * @param {Readonly<HastRoot>} node
 *   hast root to transform.
 * @returns {MdastRoot}
 *   mdast node.
 */
export function root(state, node) {
  let children = state.all(node)

  if (state.options.document || wrapNeeded(children)) {
    children = wrap(children)
  }

  /** @type {MdastRoot} */
  const result = {type: 'root', children}
  state.patch(node, result)
  return result
}
