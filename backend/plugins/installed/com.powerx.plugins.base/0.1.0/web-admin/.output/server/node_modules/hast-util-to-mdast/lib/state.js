/**
 * @import {Element, Nodes, Parents} from 'hast'
 * @import {
 *   BlockContent as MdastBlockContent,
 *   DefinitionContent as MdastDefinitionContent,
 *   Nodes as MdastNodes,
 *   Parents as MdastParents,
 *   RootContent as MdastRootContent
 * } from 'mdast'
 */

/**
 * @typedef {MdastBlockContent | MdastDefinitionContent} MdastFlowContent
 */

/**
 * @callback All
 *   Transform the children of a hast parent to mdast.
 * @param {Parents} parent
 *   Parent.
 * @returns {Array<MdastRootContent>}
 *   mdast children.
 *
 * @callback Handle
 *   Handle a particular element.
 * @param {State} state
 *   Info passed around about the current state.
 * @param {Element} element
 *   Element to transform.
 * @param {Parents | undefined} parent
 *   Parent of `element`.
 * @returns {Array<MdastNodes> | MdastNodes | undefined | void}
 *   mdast node or nodes.
 *
 *   Note: `void` is included until TS nicely infers `undefined`.
 *
 * @callback NodeHandle
 *   Handle a particular node.
 * @param {State} state
 *   Info passed around about the current state.
 * @param {any} node
 *   Node to transform.
 * @param {Parents | undefined} parent
 *   Parent of `node`.
 * @returns {Array<MdastNodes> | MdastNodes | undefined | void}
 *   mdast node or nodes.
 *
 *   Note: `void` is included until TS nicely infers `undefined`.
 *
 * @callback One
 *   Transform a hast node to mdast.
 * @param {Nodes} node
 *   Expected hast node.
 * @param {Parents | undefined} parent
 *   Parent of `node`.
 * @returns {Array<MdastNodes> | MdastNodes | undefined}
 *   mdast result.
 *
 * @typedef Options
 *   Configuration.
 * @property {string | null | undefined} [checked='[x]']
 *   Value to use for a checked checkbox or radio input (default: `'[x]'`)
 * @property {boolean | null | undefined} [document]
 *   Whether the given tree represents a complete document (optional).
 *
 *   Applies when the `tree` is a `root` node.
 *   When the tree represents a complete document, then things are wrapped in
 *   paragraphs when needed, and otherwise they’re left as-is.
 *   The default checks for whether there’s mixed content: some phrasing nodes
 *   *and* some non-phrasing nodes.
 * @property {Record<string, Handle | null | undefined> | null | undefined} [handlers]
 *   Object mapping tag names to functions handling the corresponding elements
 *   (optional).
 *
 *   Merged into the defaults.
 * @property {boolean | null | undefined} [newlines=false]
 *   Keep line endings when collapsing whitespace (default: `false`).
 *
 *   The default collapses to a single space.
 * @property {Record<string, NodeHandle | null | undefined> | null | undefined} [nodeHandlers]
 *   Object mapping node types to functions handling the corresponding nodes
 *   (optional).
 *
 *   Merged into the defaults.
 * @property {Array<string> | null | undefined} [quotes=['"']]
 *   List of quotes to use (default: `['"']`).
 *
 *   Each value can be one or two characters.
 *   When two, the first character determines the opening quote and the second
 *   the closing quote at that level.
 *   When one, both the opening and closing quote are that character.
 *
 *   The order in which the preferred quotes appear determines which quotes to
 *   use at which level of nesting.
 *   So, to prefer `‘’` at the first level of nesting, and `“”` at the second,
 *   pass `['‘’', '“”']`.
 *   If `<q>`s are nested deeper than the given amount of quotes, the markers
 *   wrap around: a third level of nesting when using `['«»', '‹›']` should
 *   have double guillemets, a fourth single, a fifth double again, etc.
 * @property {string | null | undefined} [unchecked='[ ]']
 *   Value to use for an unchecked checkbox or radio input (default: `'[ ]'`).
 *
 * @callback Patch
 *   Copy a node’s positional info.
 * @param {Nodes} from
 *   hast node to copy from.
 * @param {MdastNodes} to
 *   mdast node to copy into.
 * @returns {undefined}
 *   Nothing.
 *
 * @callback Resolve
 *   Resolve a URL relative to a base.
 * @param {string | null | undefined} url
 *   Possible URL value.
 * @returns {string}
 *   URL, resolved to a `base` element, if any.
 *
 * @typedef State
 *   Info passed around about the current state.
 * @property {All} all
 *   Transform the children of a hast parent to mdast.
 * @property {boolean} baseFound
 *   Whether a `<base>` element was seen.
 * @property {Map<string, Element>} elementById
 *   Elements by their `id`.
 * @property {string | undefined} frozenBaseUrl
 *   `href` of `<base>`, if any.
 * @property {Record<string, Handle>} handlers
 *   Applied element handlers.
 * @property {boolean} inTable
 *   Whether we’re in a table.
 * @property {Record<string, NodeHandle>} nodeHandlers
 *   Applied node handlers.
 * @property {One} one
 *   Transform a hast node to mdast.
 * @property {Options} options
 *   User configuration.
 * @property {Patch} patch
 *   Copy a node’s positional info.
 * @property {number} qNesting
 *   Non-negative finite integer representing how deep we’re in `<q>`s.
 * @property {Resolve} resolve
 *   Resolve a URL relative to a base.
 * @property {ToFlow} toFlow
 *   Transform a list of mdast nodes to flow.
 * @property {<ChildType extends MdastNodes, ParentType extends MdastParents & {'children': Array<ChildType>}>(nodes: Array<MdastRootContent>, build: (() => ParentType)) => Array<ParentType>} toSpecificContent
 *   Turn arbitrary content into a list of a particular node type.
 *
 *   This is useful for example for lists, which must have list items as
 *   content.
 *   in this example, when non-items are found, they will be queued, and
 *   inserted into an adjacent item.
 *   When no actual items exist, one will be made with `build`.
 *
 * @callback ToFlow
 *   Transform a list of mdast nodes to flow.
 * @param {Array<MdastRootContent>} nodes
 *   mdast nodes.
 * @returns {Array<MdastFlowContent>}
 *   mdast flow children.
 */

import {position} from 'unist-util-position'
import {handlers, nodeHandlers} from './handlers/index.js'
import {wrap} from './util/wrap.js'

const own = {}.hasOwnProperty

/**
 * Create a state.
 *
 * @param {Readonly<Options>} options
 *   User configuration.
 * @returns {State}
 *   State.
 */
export function createState(options) {
  return {
    all,
    baseFound: false,
    elementById: new Map(),
    frozenBaseUrl: undefined,
    handlers: {...handlers, ...options.handlers},
    inTable: false,
    nodeHandlers: {...nodeHandlers, ...options.nodeHandlers},
    one,
    options,
    patch,
    qNesting: 0,
    resolve,
    toFlow,
    toSpecificContent
  }
}

/**
 * Transform the children of a hast parent to mdast.
 *
 * You might want to combine this with `toFlow` or `toSpecificContent`.
 *
 * @this {State}
 *   Info passed around about the current state.
 * @param {Parents} parent
 *   Parent.
 * @returns {Array<MdastRootContent>}
 *   mdast children.
 */
function all(parent) {
  const children = parent.children || []
  /** @type {Array<MdastRootContent>} */
  const results = []
  let index = -1

  while (++index < children.length) {
    const child = children[index]
    // Content -> content.
    const result =
      /** @type {Array<MdastRootContent> | MdastRootContent | undefined} */ (
        this.one(child, parent)
      )

    if (Array.isArray(result)) {
      results.push(...result)
    } else if (result) {
      results.push(result)
    }
  }

  return results
}

/**
 * Transform a hast node to mdast.
 *
 * @this {State}
 *   Info passed around about the current state.
 * @param {Nodes} node
 *   hast node to transform.
 * @param {Parents | undefined} parent
 *   Parent of `node`.
 * @returns {Array<MdastNodes> | MdastNodes | undefined}
 *   mdast result.
 */
function one(node, parent) {
  if (node.type === 'element') {
    if (node.properties && node.properties.dataMdast === 'ignore') {
      return
    }

    if (own.call(this.handlers, node.tagName)) {
      return this.handlers[node.tagName](this, node, parent) || undefined
    }
  } else if (own.call(this.nodeHandlers, node.type)) {
    return this.nodeHandlers[node.type](this, node, parent) || undefined
  }

  // Unknown literal.
  if ('value' in node && typeof node.value === 'string') {
    /** @type {MdastRootContent} */
    const result = {type: 'text', value: node.value}
    this.patch(node, result)
    return result
  }

  // Unknown parent.
  if ('children' in node) {
    return this.all(node)
  }
}

/**
 * Copy a node’s positional info.
 *
 * @param {Nodes} origin
 *   hast node to copy from.
 * @param {MdastNodes} node
 *   mdast node to copy into.
 * @returns {undefined}
 *   Nothing.
 */
function patch(origin, node) {
  if (origin.position) node.position = position(origin)
}

/**
 * @this {State}
 *   Info passed around about the current state.
 * @param {string | null | undefined} url
 *   Possible URL value.
 * @returns {string}
 *   URL, resolved to a `base` element, if any.
 */
function resolve(url) {
  const base = this.frozenBaseUrl

  if (url === null || url === undefined) {
    return ''
  }

  if (base) {
    return String(new URL(url, base))
  }

  return url
}

/**
 * Transform a list of mdast nodes to flow.
 *
 * @this {State}
 *   Info passed around about the current state.
 * @param {Array<MdastRootContent>} nodes
 *   Parent.
 * @returns {Array<MdastFlowContent>}
 *   mdast flow children.
 */
function toFlow(nodes) {
  return wrap(nodes)
}

/**
 * Turn arbitrary content into a particular node type.
 *
 * This is useful for example for lists, which must have list items as content.
 * in this example, when non-items are found, they will be queued, and
 * inserted into an adjacent item.
 * When no actual items exist, one will be made with `build`.
 *
 * @template {MdastNodes} ChildType
 *   Node type of children.
 * @template {MdastParents & {'children': Array<ChildType>}} ParentType
 *   Node type of parent.
 * @param {Array<MdastRootContent>} nodes
 *   Nodes, which are either `ParentType`, or will be wrapped in one.
 * @param {() => ParentType} build
 *   Build a parent if needed (must have empty `children`).
 * @returns {Array<ParentType>}
 *   List of parents.
 */
function toSpecificContent(nodes, build) {
  const reference = build()
  /** @type {Array<ParentType>} */
  const results = []
  /** @type {Array<ChildType>} */
  let queue = []
  let index = -1

  while (++index < nodes.length) {
    const node = nodes[index]

    if (expectedParent(node)) {
      if (queue.length > 0) {
        node.children.unshift(...queue)
        queue = []
      }

      results.push(node)
    } else {
      // Assume `node` can be a child of `ParentType`.
      // If we start checking nodes, we’d run into problems with unknown nodes,
      // which we do want to support.
      const child = /** @type {ChildType} */ (node)
      queue.push(child)
    }
  }

  if (queue.length > 0) {
    let node = results[results.length - 1]

    if (!node) {
      node = build()
      results.push(node)
    }

    node.children.push(...queue)
    queue = []
  }

  return results

  /**
   * @param {MdastNodes} node
   * @returns {node is ParentType}
   */
  function expectedParent(node) {
    return node.type === reference.type
  }
}
