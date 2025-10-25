/**
 * @import {} from 'mdast-util-to-hast'
 * @import {
 *   BlockContent,
 *   Delete,
 *   Link,
 *   Nodes,
 *   Paragraph,
 *   Parents,
 *   PhrasingContent,
 *   RootContent
 * } from 'mdast'
 */

import structuredClone from '@ungap/structured-clone'
import {phrasing as hastPhrasing} from 'hast-util-phrasing'
import {whitespace} from 'hast-util-whitespace'
import {phrasing as mdastPhrasing} from 'mdast-util-phrasing'
import {dropSurroundingBreaks} from './drop-surrounding-breaks.js'

/**
 * Check if there are phrasing mdast nodes.
 *
 * This is needed if a fragment is given, which could just be a sentence, and
 * doesn’t need a wrapper paragraph.
 *
 * @param {Array<Nodes>} nodes
 * @returns {boolean}
 */
export function wrapNeeded(nodes) {
  let index = -1

  while (++index < nodes.length) {
    const node = nodes[index]

    if (!phrasing(node) || ('children' in node && wrapNeeded(node.children))) {
      return true
    }
  }

  return false
}

/**
 * Wrap runs of phrasing content into paragraphs, leaving the non-phrasing
 * content as-is.
 *
 * @param {Array<RootContent>} nodes
 *   Content.
 * @returns {Array<BlockContent>}
 *   Content where phrasing is wrapped in paragraphs.
 */
export function wrap(nodes) {
  return runs(nodes, onphrasing, function (d) {
    return d
  })

  /**
   * @param {Array<PhrasingContent>} nodes
   * @returns {Array<Paragraph>}
   */
  function onphrasing(nodes) {
    return nodes.every(function (d) {
      return d.type === 'text' ? whitespace(d.value) : false
    })
      ? []
      : [{type: 'paragraph', children: dropSurroundingBreaks(nodes)}]
  }
}

/**
 * @param {Delete | Link} node
 * @returns {Array<BlockContent>}
 */
function split(node) {
  return runs(node.children, onphrasing, onnonphrasing)

  /**
   * Use `parent`, put the phrasing run inside it.
   *
   * @param {Array<PhrasingContent>} nodes
   * @returns {Array<BlockContent>}
   */
  function onphrasing(nodes) {
    const newParent = cloneWithoutChildren(node)
    newParent.children = nodes
    // @ts-expect-error Assume fine.
    return [newParent]
  }

  /**
   * Use `child`, add `parent` as its first child, put the original children
   * into `parent`.
   * If `child` is not a parent, `parent` will not be added.
   *
   * @param {BlockContent} child
   * @returns {BlockContent}
   */
  function onnonphrasing(child) {
    if ('children' in child && 'children' in node) {
      const newParent = cloneWithoutChildren(node)
      const newChild = cloneWithoutChildren(child)
      // @ts-expect-error Assume fine.
      newParent.children = child.children
      // @ts-expect-error Assume fine.
      newChild.children.push(newParent)
      return newChild
    }

    return {...child}
  }
}

/**
 * Wrap all runs of mdast phrasing content in `paragraph` nodes.
 *
 * @param {Array<RootContent>} nodes
 *   List of input nodes.
 * @param {(nodes: Array<PhrasingContent>) => Array<BlockContent>} onphrasing
 *   Turn phrasing content into block content.
 * @param {(node: BlockContent) => BlockContent} onnonphrasing
 *   Map block content (defaults to keeping them as-is).
 * @returns {Array<BlockContent>}
 */
function runs(nodes, onphrasing, onnonphrasing) {
  const flattened = flatten(nodes)
  /** @type {Array<BlockContent>} */
  const result = []
  /** @type {Array<PhrasingContent>} */
  let queue = []
  let index = -1

  while (++index < flattened.length) {
    const node = flattened[index]

    if (phrasing(node)) {
      queue.push(node)
    } else {
      if (queue.length > 0) {
        result.push(...onphrasing(queue))
        queue = []
      }

      // @ts-expect-error Assume non-phrasing.
      result.push(onnonphrasing(node))
    }
  }

  if (queue.length > 0) {
    result.push(...onphrasing(queue))
    queue = []
  }

  return result
}

/**
 * Flatten a list of nodes.
 *
 * @param {Array<RootContent>} nodes
 *   List of nodes, will unravel `delete` and `link`.
 * @returns {Array<RootContent>}
 *   Unraveled nodes.
 */
function flatten(nodes) {
  /** @type {Array<RootContent>} */
  const flattened = []
  let index = -1

  while (++index < nodes.length) {
    const node = nodes[index]

    // Straddling: some elements are *weird*.
    // Namely: `map`, `ins`, `del`, and `a`, as they are hybrid elements.
    // See: <https://html.spec.whatwg.org/#paragraphs>.
    // Paragraphs are the weirdest of them all.
    // See the straddling fixture for more info!
    // `ins` is ignored in mdast, so we don’t need to worry about that.
    // `map` maps to its content, so we don’t need to worry about that either.
    // `del` maps to `delete` and `a` to `link`, so we do handle those.
    // What we’ll do is split `node` over each of its children.
    if (
      (node.type === 'delete' || node.type === 'link') &&
      wrapNeeded(node.children)
    ) {
      flattened.push(...split(node))
    } else {
      flattened.push(node)
    }
  }

  return flattened
}

/**
 * Check if an mdast node is phrasing.
 *
 * Also supports checking embedded hast fields.
 *
 * @param {Nodes} node
 *   mdast node to check.
 * @returns {node is PhrasingContent}
 *   Whether `node` is phrasing content (includes nodes with `hName` fields
 *   set to phrasing hast element names).
 */
function phrasing(node) {
  const tagName = node.data && node.data.hName
  return tagName
    ? hastPhrasing({type: 'element', tagName, properties: {}, children: []})
    : mdastPhrasing(node)
}

/**
 * @template {Parents} ParentType
 *   Parent type.
 * @param {ParentType} node
 *   Node to clone.
 * @returns {ParentType}
 *   Cloned node, without children.
 */
function cloneWithoutChildren(node) {
  return structuredClone({...node, children: []})
}
