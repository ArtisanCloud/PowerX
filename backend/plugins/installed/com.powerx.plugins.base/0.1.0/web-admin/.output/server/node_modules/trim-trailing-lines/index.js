/**
 * Remove final line endings from `value`
 *
 * @param {unknown} value
 *   Value with trailing line endings, coerced to string.
 * @return {string}
 *   Value without trailing line endings.
 */
export function trimTrailingLines(value) {
  const input = String(value)
  let end = input.length

  while (end > 0) {
    const code = input.codePointAt(end - 1)
    if (code !== undefined && (code === 10 || code === 13)) {
      end--
    } else {
      break
    }
  }

  return input.slice(0, end)
}
