/**
 * @import {State} from 'hast-util-to-mdast'
 * @import {Element} from 'hast'
 * @import {Link} from 'mdast'
 */

/**
 * @param {State} state
 *   State.
 * @param {Readonly<Element>} node
 *   hast element to transform.
 * @returns {Link | undefined}
 *   mdast node.
 */
export function iframe(state, node) {
  const properties = node.properties || {}
  const source = String(properties.src || '')
  const title = String(properties.title || '')

  // Only create a link if there is a title.
  // We can’t use the content of the frame because conforming HTML parsers treat
  // it as text, whereas legacy parsers treat it as HTML, so it will likely
  // contain tags that will show up in text.
  if (source && title) {
    /** @type {Link} */
    const result = {
      type: 'link',
      title: null,
      url: state.resolve(source),
      children: [{type: 'text', value: title}]
    }
    state.patch(node, result)
    return result
  }
}
