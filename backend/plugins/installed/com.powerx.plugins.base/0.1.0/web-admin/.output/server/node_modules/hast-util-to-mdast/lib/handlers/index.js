/**
 * @import {State} from 'hast-util-to-mdast'
 * @import {Parents} from 'hast'
 */

import {a} from './a.js'
import {base} from './base.js'
import {blockquote} from './blockquote.js'
import {br} from './br.js'
import {code} from './code.js'
import {comment} from './comment.js'
import {del} from './del.js'
import {dl} from './dl.js'
import {em} from './em.js'
import {heading} from './heading.js'
import {hr} from './hr.js'
import {iframe} from './iframe.js'
import {img} from './img.js'
import {inlineCode} from './inline-code.js'
import {input} from './input.js'
import {li} from './li.js'
import {list} from './list.js'
import {media} from './media.js'
import {p} from './p.js'
import {q} from './q.js'
import {root} from './root.js'
import {select} from './select.js'
import {strong} from './strong.js'
import {tableCell} from './table-cell.js'
import {tableRow} from './table-row.js'
import {table} from './table.js'
import {text} from './text.js'
import {textarea} from './textarea.js'
import {wbr} from './wbr.js'

/**
 * Default handlers for nodes.
 *
 * Each key is a node type, each value is a `NodeHandler`.
 */
export const nodeHandlers = {
  comment,
  doctype: ignore,
  root,
  text
}

/**
 * Default handlers for elements.
 *
 * Each key is an element name, each value is a `Handler`.
 */
export const handlers = {
  // Ignore:
  applet: ignore,
  area: ignore,
  basefont: ignore,
  bgsound: ignore,
  caption: ignore,
  col: ignore,
  colgroup: ignore,
  command: ignore,
  content: ignore,
  datalist: ignore,
  dialog: ignore,
  element: ignore,
  embed: ignore,
  frame: ignore,
  frameset: ignore,
  isindex: ignore,
  keygen: ignore,
  link: ignore,
  math: ignore,
  menu: ignore,
  menuitem: ignore,
  meta: ignore,
  nextid: ignore,
  noembed: ignore,
  noframes: ignore,
  optgroup: ignore,
  option: ignore,
  param: ignore,
  script: ignore,
  shadow: ignore,
  source: ignore,
  spacer: ignore,
  style: ignore,
  svg: ignore,
  template: ignore,
  title: ignore,
  track: ignore,

  // Use children:
  abbr: all,
  acronym: all,
  bdi: all,
  bdo: all,
  big: all,
  blink: all,
  button: all,
  canvas: all,
  cite: all,
  data: all,
  details: all,
  dfn: all,
  font: all,
  ins: all,
  label: all,
  map: all,
  marquee: all,
  meter: all,
  nobr: all,
  noscript: all,
  object: all,
  output: all,
  progress: all,
  rb: all,
  rbc: all,
  rp: all,
  rt: all,
  rtc: all,
  ruby: all,
  slot: all,
  small: all,
  span: all,
  sup: all,
  sub: all,
  tbody: all,
  tfoot: all,
  thead: all,
  time: all,

  // Use children as flow.
  address: flow,
  article: flow,
  aside: flow,
  body: flow,
  center: flow,
  div: flow,
  fieldset: flow,
  figcaption: flow,
  figure: flow,
  form: flow,
  footer: flow,
  header: flow,
  hgroup: flow,
  html: flow,
  legend: flow,
  main: flow,
  multicol: flow,
  nav: flow,
  picture: flow,
  section: flow,

  // Handle.
  a,
  audio: media,
  b: strong,
  base,
  blockquote,
  br,
  code: inlineCode,
  dir: list,
  dl,
  dt: li,
  dd: li,
  del,
  em,
  h1: heading,
  h2: heading,
  h3: heading,
  h4: heading,
  h5: heading,
  h6: heading,
  hr,
  i: em,
  iframe,
  img,
  image: img,
  input,
  kbd: inlineCode,
  li,
  listing: code,
  mark: em,
  ol: list,
  p,
  plaintext: code,
  pre: code,
  q,
  s: del,
  samp: inlineCode,
  select,
  strike: del,
  strong,
  summary: p,
  table,
  td: tableCell,
  textarea,
  th: tableCell,
  tr: tableRow,
  tt: inlineCode,
  u: em,
  ul: list,
  var: inlineCode,
  video: media,
  wbr,
  xmp: code
}

/**
 * @param {State} state
 *   State.
 * @param {Parents} node
 *   Parent to transform.
 */
function all(state, node) {
  return state.all(node)
}

/**
 * @param {State} state
 *   State.
 * @param {Parents} node
 *   Parent to transform.
 */
function flow(state, node) {
  return state.toFlow(state.all(node))
}

/**
 * @returns {undefined}
 */
function ignore() {}
