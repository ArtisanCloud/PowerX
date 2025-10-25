/**
 * @import {ListContent} from 'mdast'
 */

/**
 * Infer whether list items are spread.
 *
 * @param {Readonly<Array<Readonly<ListContent>>>} children
 *   List items.
 * @returns {boolean}
 *   Whether one or more list items are spread.
 */
export function listItemsSpread(children) {
  let index = -1

  if (children.length > 1) {
    while (++index < children.length) {
      if (children[index].spread) {
        return true
      }
    }
  }

  return false
}
