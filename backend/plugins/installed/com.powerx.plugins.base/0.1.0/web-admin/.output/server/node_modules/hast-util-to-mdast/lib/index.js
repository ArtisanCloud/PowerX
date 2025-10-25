/**
 * @import {Options} from 'hast-util-to-mdast'
 * @import {Nodes} from 'hast'
 * @import {Nodes as MdastNodes, RootContent as MdastRootContent} from 'mdast'
 */

import structuredClone from '@ungap/structured-clone'
import rehypeMinifyWhitespace from 'rehype-minify-whitespace'
import {visit} from 'unist-util-visit'
import {createState} from './state.js'

/** @type {Readonly<Options>} */
const emptyOptions = {}

/**
 * Transform hast to mdast.
 *
 * @param {Readonly<Nodes>} tree
 *   hast tree to transform.
 * @param {Readonly<Options> | null | undefined} [options]
 *   Configuration (optional).
 * @returns {MdastNodes}
 *   mdast tree.
 */
export function toMdast(tree, options) {
  // We have to clone, cause we’ll use `rehype-minify-whitespace` on the tree,
  // which modifies.
  const cleanTree = structuredClone(tree)
  const settings = options || emptyOptions
  const transformWhitespace = rehypeMinifyWhitespace({
    newlines: settings.newlines === true
  })
  const state = createState(settings)
  /** @type {MdastNodes} */
  let mdast

  // @ts-expect-error: fine to pass an arbitrary node.
  transformWhitespace(cleanTree)

  visit(cleanTree, function (node) {
    if (node && node.type === 'element' && node.properties) {
      const id = String(node.properties.id || '') || undefined

      if (id && !state.elementById.has(id)) {
        state.elementById.set(id, node)
      }
    }
  })

  const result = state.one(cleanTree, undefined)

  if (!result) {
    mdast = {type: 'root', children: []}
  } else if (Array.isArray(result)) {
    // Assume content.
    const children = /** @type {Array<MdastRootContent>} */ (result)
    mdast = {type: 'root', children}
  } else {
    mdast = result
  }

  // Collapse text nodes, and fix whitespace.
  //
  // Most of this is taken care of by `rehype-minify-whitespace`, but
  // we’re generating some whitespace too, and some nodes are in the end
  // ignored.
  // So clean up.
  visit(mdast, function (node, index, parent) {
    if (node.type === 'text' && index !== undefined && parent) {
      const previous = parent.children[index - 1]

      if (previous && previous.type === node.type) {
        previous.value += node.value
        parent.children.splice(index, 1)

        if (previous.position && node.position) {
          previous.position.end = node.position.end
        }

        // Iterate over the previous node again, to handle its total value.
        return index - 1
      }

      node.value = node.value.replace(/[\t ]*(\r?\n|\r)[\t ]*/, '$1')

      // We don’t care about other phrasing nodes in between (e.g., `[ asd ]()`),
      // as there the whitespace matters.
      if (
        parent &&
        (parent.type === 'heading' ||
          parent.type === 'paragraph' ||
          parent.type === 'root')
      ) {
        if (!index) {
          node.value = node.value.replace(/^[\t ]+/, '')
        }

        if (index === parent.children.length - 1) {
          node.value = node.value.replace(/[\t ]+$/, '')
        }
      }

      if (!node.value) {
        parent.children.splice(index, 1)
        return index
      }
    }
  })

  return mdast
}
