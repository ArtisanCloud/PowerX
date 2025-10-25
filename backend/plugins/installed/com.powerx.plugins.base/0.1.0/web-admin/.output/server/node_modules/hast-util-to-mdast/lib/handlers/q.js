/**
 * @import {State} from 'hast-util-to-mdast'
 * @import {Element} from 'hast'
 * @import {RootContent as MdastRootContent} from 'mdast'
 */

const defaultQuotes = ['"']

/**
 * @param {State} state
 *   State.
 * @param {Readonly<Element>} node
 *   hast element to transform.
 * @returns {Array<MdastRootContent>}
 *   mdast nodes.
 */
export function q(state, node) {
  const quotes = state.options.quotes || defaultQuotes

  state.qNesting++
  const contents = state.all(node)
  state.qNesting--

  const quote = quotes[state.qNesting % quotes.length]
  const head = contents[0]
  const tail = contents[contents.length - 1]
  const open = quote.charAt(0)
  const close = quote.length > 1 ? quote.charAt(1) : quote

  if (head && head.type === 'text') {
    head.value = open + head.value
  } else {
    contents.unshift({type: 'text', value: open})
  }

  if (tail && tail.type === 'text') {
    tail.value += close
  } else {
    contents.push({type: 'text', value: close})
  }

  return contents
}
