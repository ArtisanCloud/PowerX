/**
 * @typedef {import('./lib/state.js').Handle} Handle
 * @typedef {import('./lib/state.js').NodeHandle} NodeHandle
 * @typedef {import('./lib/state.js').Options} Options
 * @typedef {import('./lib/state.js').State} State
 */

export {toMdast} from './lib/index.js'

export {
  handlers as defaultHandlers,
  nodeHandlers as defaultNodeHandlers
} from './lib/handlers/index.js'
