/**
 * @import {State} from 'hast-util-to-mdast'
 * @import {Element} from 'hast'
 * @import {AlignType, RowContent, TableContent, Table, Text} from 'mdast'
 */

/**
 * @typedef Info
 *   Inferred info on a table.
 * @property {Array<AlignType>} align
 *   Alignment.
 * @property {boolean} headless
 *   Whether a `thead` is missing.
 */

import {toText} from 'hast-util-to-text'
import {SKIP, visit} from 'unist-util-visit'

/**
 * @param {State} state
 *   State.
 * @param {Readonly<Element>} node
 *   hast element to transform.
 * @returns {Table | Text}
 *   mdast node.
 */
// eslint-disable-next-line complexity
export function table(state, node) {
  // Ignore nested tables.
  if (state.inTable) {
    /** @type {Text} */
    const result = {type: 'text', value: toText(node)}
    state.patch(node, result)
    return result
  }

  state.inTable = true

  const {align, headless} = inspect(node)
  const rows = state.toSpecificContent(state.all(node), createRow)

  // Add an empty header row.
  if (headless) {
    rows.unshift(createRow())
  }

  let rowIndex = -1

  while (++rowIndex < rows.length) {
    const row = rows[rowIndex]
    const cells = state.toSpecificContent(row.children, createCell)
    row.children = cells
  }

  let columns = 1
  rowIndex = -1

  while (++rowIndex < rows.length) {
    const cells = rows[rowIndex].children
    let cellIndex = -1

    while (++cellIndex < cells.length) {
      const cell = cells[cellIndex]

      if (cell.data) {
        const data = /** @type {Record<string, unknown>} */ (cell.data)
        const colSpan =
          Number.parseInt(String(data.hastUtilToMdastTemporaryColSpan), 10) || 1
        const rowSpan =
          Number.parseInt(String(data.hastUtilToMdastTemporaryRowSpan), 10) || 1

        if (colSpan > 1 || rowSpan > 1) {
          let otherRowIndex = rowIndex - 1

          while (++otherRowIndex < rowIndex + rowSpan) {
            let colIndex = cellIndex - 1

            while (++colIndex < cellIndex + colSpan) {
              if (!rows[otherRowIndex]) {
                // Don’t add rows that don’t exist.
                // Browsers don’t render them either.
                break
              }

              /** @type {Array<RowContent>} */
              const newCells = []

              if (otherRowIndex !== rowIndex || colIndex !== cellIndex) {
                newCells.push({type: 'tableCell', children: []})
              }

              rows[otherRowIndex].children.splice(colIndex, 0, ...newCells)
            }
          }
        }

        // Clean the data fields.
        if ('hastUtilToMdastTemporaryColSpan' in cell.data)
          delete cell.data.hastUtilToMdastTemporaryColSpan
        if ('hastUtilToMdastTemporaryRowSpan' in cell.data)
          delete cell.data.hastUtilToMdastTemporaryRowSpan
        if (Object.keys(cell.data).length === 0) delete cell.data
      }
    }

    if (cells.length > columns) columns = cells.length
  }

  // Add extra empty cells.
  rowIndex = -1

  while (++rowIndex < rows.length) {
    const cells = rows[rowIndex].children
    let cellIndex = cells.length - 1
    while (++cellIndex < columns) {
      cells.push({type: 'tableCell', children: []})
    }
  }

  let alignIndex = align.length - 1
  while (++alignIndex < columns) {
    align.push(null)
  }

  state.inTable = false

  /** @type {Table} */
  const result = {type: 'table', align, children: rows}
  state.patch(node, result)
  return result
}

/**
 * Infer whether the HTML table has a head and how it aligns.
 *
 * @param {Readonly<Element>} node
 *   Table element to check.
 * @returns {Info}
 *   Info.
 */
function inspect(node) {
  /** @type {Info} */
  const info = {align: [null], headless: true}
  let rowIndex = 0
  let cellIndex = 0

  visit(node, function (child) {
    if (child.type === 'element') {
      // Don’t enter nested tables.
      if (child.tagName === 'table' && node !== child) {
        return SKIP
      }

      if (
        (child.tagName === 'th' || child.tagName === 'td') &&
        child.properties
      ) {
        if (!info.align[cellIndex]) {
          const value = String(child.properties.align || '') || null

          if (
            value === 'center' ||
            value === 'left' ||
            value === 'right' ||
            value === null
          ) {
            info.align[cellIndex] = value
          }
        }

        // If there is a `th` in the first row, assume there is a header row.
        if (info.headless && rowIndex < 2 && child.tagName === 'th') {
          info.headless = false
        }

        cellIndex++
      }
      // If there is a `thead`, assume there is a header row.
      else if (child.tagName === 'thead') {
        info.headless = false
      } else if (child.tagName === 'tr') {
        rowIndex++
        cellIndex = 0
      }
    }
  })

  return info
}

/**
 * @returns {RowContent}
 */
function createCell() {
  return {type: 'tableCell', children: []}
}

/**
 * @returns {TableContent}
 */
function createRow() {
  return {type: 'tableRow', children: []}
}
