/**
 * @import {State} from 'hast-util-to-mdast'
 * @import {Element} from 'hast'
 * @import {ListItem} from 'mdast'
 */

/**
 * @typedef ExtractResult
 *   Result of extracting a leading checkbox.
 * @property {Element | undefined} checkbox
 *   The checkbox that was removed, if any.
 * @property {Element} rest
 *   If there was a leading checkbox, a deep clone of the node w/o the leading
 *   checkbox; otherwise a reference to the given, untouched, node.
 */

import {phrasing} from 'hast-util-phrasing'

/**
 * @param {State} state
 *   State.
 * @param {Readonly<Element>} node
 *   hast element to transform.
 * @returns {ListItem}
 *   mdast node.
 */
export function li(state, node) {
  // If the list item starts with a checkbox, remove the checkbox and mark the
  // list item as a GFM task list item.
  const {rest, checkbox} = extractLeadingCheckbox(node)
  const checked = checkbox ? Boolean(checkbox.properties.checked) : null
  const spread = spreadout(rest)
  const children = state.toFlow(state.all(rest))

  /** @type {ListItem} */
  const result = {type: 'listItem', spread, checked, children}
  state.patch(node, result)
  return result
}

/**
 * Check if an element should spread out.
 *
 * The reason to spread out a markdown list item is primarily whether writing
 * the equivalent in markdown, would yield a spread out item.
 *
 * A spread out item results in `<p>` and `</p>` tags.
 * Otherwise, the phrasing would be output directly.
 * We can check for that: if there’s a `<p>` element, spread it out.
 *
 * But what if there are no paragraphs?
 * In that case, we can also assume that if two “block” things were written in
 * an item, that it is spread out, because blocks are typically joined by blank
 * lines, which also means a spread item.
 *
 * Lastly, because in HTML things can be wrapped in a `<div>` or similar, we
 * delve into non-phrasing elements here to figure out if they themselves
 * contain paragraphs or 2 or more flow non-phrasing elements.
 *
 * @param {Readonly<Element>} node
 * @returns {boolean}
 */
function spreadout(node) {
  let index = -1
  let seenFlow = false

  while (++index < node.children.length) {
    const child = node.children[index]

    if (child.type === 'element') {
      if (phrasing(child)) continue

      if (child.tagName === 'p' || seenFlow || spreadout(child)) {
        return true
      }

      seenFlow = true
    }
  }

  return false
}

/**
 * Extract a leading checkbox from a list item.
 *
 * If there was a leading checkbox, makes a deep clone of the node w/o the
 * leading checkbox; otherwise a reference to the given, untouched, node is
 * given back.
 *
 * So for example:
 *
 * ```html
 * <li><input type="checkbox">Text</li>
 * ```
 *
 * …becomes:
 *
 * ```html
 * <li>Text</li>
 * ```
 *
 * ```html
 * <li><p><input type="checkbox">Text</p></li>
 * ```
 *
 * …becomes:
 *
 * ```html
 * <li><p>Text</p></li>
 * ```
 *
 * @param {Readonly<Element>} node
 * @returns {ExtractResult}
 */
function extractLeadingCheckbox(node) {
  const head = node.children[0]

  if (
    head &&
    head.type === 'element' &&
    head.tagName === 'input' &&
    head.properties &&
    (head.properties.type === 'checkbox' || head.properties.type === 'radio')
  ) {
    const rest = {...node, children: node.children.slice(1)}
    return {checkbox: head, rest}
  }

  // The checkbox may be nested in another element.
  // If the first element has children, look for a leading checkbox inside it.
  //
  // This only handles nesting in `<p>` elements, which is most common.
  // It’s possible a leading checkbox might be nested in other types of flow or
  // phrasing elements (and *deeply* nested, which is not possible with `<p>`).
  // Limiting things to `<p>` elements keeps this simpler for now.
  if (head && head.type === 'element' && head.tagName === 'p') {
    const {checkbox, rest: restHead} = extractLeadingCheckbox(head)

    if (checkbox) {
      const rest = {...node, children: [restHead, ...node.children.slice(1)]}
      return {checkbox, rest}
    }
  }

  return {checkbox: undefined, rest: node}
}
