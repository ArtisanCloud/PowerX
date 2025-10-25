/**
 * @import {State} from 'hast-util-to-mdast'
 * @import {ElementContent, Element} from 'hast'
 * @import {BlockContent, DefinitionContent, ListContent, ListItem, List} from 'mdast'
 */

/**
 * @typedef Group
 *   Title/definition group.
 * @property {Array<Element>} titles
 *   One or more titles.
 * @property {Array<ElementContent>} definitions
 *   One or more definitions.
 */

import {listItemsSpread} from '../util/list-items-spread.js'

/**
 * @param {State} state
 *   State.
 * @param {Readonly<Element>} node
 *   hast element to transform.
 * @returns {List | undefined}
 *   mdast node.
 */
export function dl(state, node) {
  /** @type {Array<ElementContent>} */
  const clean = []
  /** @type {Array<Group>} */
  const groups = []
  let index = -1

  // Unwrap `<div>`s
  while (++index < node.children.length) {
    const child = node.children[index]

    if (child.type === 'element' && child.tagName === 'div') {
      clean.push(...child.children)
    } else {
      clean.push(child)
    }
  }

  /** @type {Group} */
  let group = {definitions: [], titles: []}
  index = -1

  // Group titles and definitions.
  while (++index < clean.length) {
    const child = clean[index]

    if (child.type === 'element' && child.tagName === 'dt') {
      const previous = clean[index - 1]

      if (
        previous &&
        previous.type === 'element' &&
        previous.tagName === 'dd'
      ) {
        groups.push(group)
        group = {definitions: [], titles: []}
      }

      group.titles.push(child)
    } else {
      group.definitions.push(child)
    }
  }

  groups.push(group)

  // Create items.
  index = -1
  /** @type {Array<ListContent>} */
  const content = []

  while (++index < groups.length) {
    const result = [
      ...handle(state, groups[index].titles),
      ...handle(state, groups[index].definitions)
    ]

    if (result.length > 0) {
      content.push({
        type: 'listItem',
        spread: result.length > 1,
        checked: null,
        children: result
      })
    }
  }

  // Create a list if there are items.
  if (content.length > 0) {
    /** @type {List} */
    const result = {
      type: 'list',
      ordered: false,
      start: null,
      spread: listItemsSpread(content),
      children: content
    }
    state.patch(node, result)
    return result
  }
}

/**
 * @param {State} state
 *   State.
 * @param {Array<ElementContent>} children
 *   hast element children to transform.
 * @returns {Array<BlockContent | DefinitionContent>}
 *   mdast nodes.
 */
function handle(state, children) {
  const nodes = state.all({type: 'root', children})
  const listItems = state.toSpecificContent(nodes, create)

  if (listItems.length === 0) {
    return []
  }

  if (listItems.length === 1) {
    return listItems[0].children
  }

  return [
    {
      type: 'list',
      ordered: false,
      start: null,
      spread: listItemsSpread(listItems),
      children: listItems
    }
  ]
}

/**
 * @returns {ListItem}
 */
function create() {
  return {type: 'listItem', spread: false, checked: null, children: []}
}
