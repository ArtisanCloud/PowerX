// 修复已包裹的 UIcon：保留 UIcon 的 class，同时在外层 span 保留类并追加 inline-block
// 不新增包裹，仅修复现有包裹。支持自闭合与非自闭合 UIcon，支持单行与多行、任意缩进。
//
// 使用：node scripts/codemods/fix-uicon-wrap.js

import fs from 'node:fs'
import path from 'node:path'

function walk(dir, list = []) {
  const entries = fs.readdirSync(dir, { withFileTypes: true })
  for (const e of entries) {
    const p = path.join(dir, e.name)
    if (e.isDirectory()) {
      if (e.name === 'node_modules' || e.name.startsWith('.nuxt') || e.name === '.output' || e.name === '.git') continue
      walk(p, list)
    } else {
      list.push(p)
    }
  }
  return list
}

function ensureInlineBlock(cls) {
  const parts = String(cls).split(/\s+/).filter(Boolean)
  if (!parts.includes('inline-block')) parts.push('inline-block')
  return dedupe(parts).join(' ')
}

function dedupe(arrOrStr) {
  const arr = Array.isArray(arrOrStr) ? arrOrStr : String(arrOrStr).split(/\s+/)
  const set = new Set(arr.filter(Boolean))
  return Array.from(set)
}

function mergeClassIntoAttrs(attrs, addClasses) {
  const add = dedupe(addClasses).join(' ')
  const re = /\bclass\s*=\s*(["'])(.*?)\1/
  const m = attrs.match(re)
  if (m) {
    const quote = m[1]
    const cur = m[2]
    const merged = dedupe((cur + ' ' + add).split(/\s+/)).join(' ')
    return attrs.replace(re, `class=${quote}${merged}${quote}`)
  } else {
    // 在属性开头插入 class，保持其余属性不变
    const prefixSpace = attrs.startsWith(' ') ? '' : ' '
    return `${prefixSpace}class="${add}"${attrs}`
  }
}

function indentBlock(indent, inner) {
  // 把内部多行统一缩进为 两个空格
  const lines = inner.split('\n')
  if (lines.length <= 1) return inner
  return lines.map((line, i) => (i === 0 ? line : indent + '  ' + line.trimStart())).join('\n')
}

function fixWrappedSelfClosing(content) {
  // 匹配：
  // <span class="SC">  [可含空白/换行]
  //   <UIcon ... />     [任意缩进]
  // </span>
  const re = /(^[ \t]*)<span\s+class="([^"]*)">\s*<UIcon\b([\s\S]*?)\/>\s*<\/span>/gm
  let changed = false

  const out = content.replace(re, (match, indent, spanClass, uiAttrs) => {
    const newSpanClass = ensureInlineBlock(spanClass)
    const newUiAttrs = mergeClassIntoAttrs(uiAttrs, spanClass) // 把 span 的类也赋给 UIcon

    changed = true
    const inner = `<UIcon${newUiAttrs} />`
    const innerIndented = indentBlock(indent, inner)
    return `${indent}<span class="${newSpanClass}">\n${indent}  ${innerIndented}\n${indent}</span>`
  })
  return { result: out, changed }
}

function fixWrappedNonSelfClosing(content) {
  // 匹配：
  // <span class="SC">  [可含空白/换行]
  //   <UIcon ...> ... </UIcon>
  // </span>
  const re = /(^[ \t]*)<span\s+class="([^"]*)">\s*<UIcon\b([\s\S]*?)>([\s\S]*?)<\/UIcon>\s*<\/span>/gm
  let changed = false

  const out = content.replace(re, (match, indent, spanClass, uiAttrs, inner) => {
    const newSpanClass = ensureInlineBlock(spanClass)
    const newUiAttrs = mergeClassIntoAttrs(uiAttrs, spanClass)

    changed = true
    const innerBlock = `<UIcon${newUiAttrs}>${inner}</UIcon>`
    const innerIndented = indentBlock(indent, innerBlock)
    return `${indent}<span class="${newSpanClass}">\n${indent}  ${innerIndented}\n${indent}</span>`
  })
  return { result: out, changed }
}

function transformContent(content) {
  let any = false
  let step = content

  const r1 = fixWrappedSelfClosing(step)
  any = any || r1.changed
  step = r1.result

  const r2 = fixWrappedNonSelfClosing(step)
  any = any || r2.changed
  step = r2.result

  return { result: step, changed: any }
}

function main() {
  const root = path.resolve(process.cwd(), 'app')
  if (!fs.existsSync(root)) {
    console.error('未找到 app 目录，脚本终止。')
    process.exit(1)
  }

  const files = walk(root).filter(f => f.endsWith('.vue'))
  const changedFiles = []

  for (const file of files) {
    const raw = fs.readFileSync(file, 'utf8')
    const { result, changed } = transformContent(raw)
    if (changed && result !== raw) {
      fs.writeFileSync(file, result, 'utf8')
      changedFiles.push(file)
    }
  }

  if (changedFiles.length === 0) {
    console.log('没有发现需要修复的已包裹 UIcon。')
  } else {
    console.log(`已修复 ${changedFiles.length} 个文件：`)
    for (const f of changedFiles) console.log(' - ' + path.relative(process.cwd(), f))
  }
}

main()