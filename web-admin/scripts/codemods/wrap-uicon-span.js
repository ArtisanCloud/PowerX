// 批量将带有静态 class 的 <UIcon ...> 包装为 <span class="... inline-block">...</span>
// 规则：
// - 仅处理静态 class="..."（不处理 :class 动态类名）；:class、v-if 等其他属性保持不变
// - 将静态 class 移至外层 span，并在 span 的 class 中追加 inline-block（若已包含则不重复）
// - 同时支持两种形式：
//    1) 自闭合：<UIcon ... class="..." />
//    2) 非自闭合：<UIcon ... class="..."> ... </UIcon>
// - 保持原有缩进与多行属性格式
//
// 使用：node scripts/codemods/wrap-uicon-span.js

import fs from 'node:fs';
import path from 'node:path';

function walk(dir, list = []) {
  const entries = fs.readdirSync(dir, { withFileTypes: true });
  for (const e of entries) {
    const p = path.join(dir, e.name);
    if (e.isDirectory()) {
      if (e.name === 'node_modules' || e.name.startsWith('.nuxt') || e.name === '.output' || e.name === '.git') continue;
      walk(p, list);
    } else {
      list.push(p);
    }
  }
  return list;
}

function buildAttrsWithoutClass(before, after) {
  const beforeTrimEnd = before.replace(/\s+$/, '');
  const afterTrimStart = after.replace(/^\s+/, '');
  return beforeTrimEnd + (beforeTrimEnd && afterTrimStart ? ' ' : '') + afterTrimStart;
}

function ensureInlineBlock(cls) {
  let newClass = (cls || '').trim();
  if (!/\binline-block\b/.test(newClass)) {
    newClass = newClass ? newClass + ' inline-block' : 'inline-block';
  }
  return newClass;
}

function transformSelfClosing(content) {
  // 自闭合：<UIcon ... class="..." ... />
  const re = /<UIcon\b([\s\S]*?)\bclass\s*=\s*(["'])(.*?)\2([\s\S]*?)\/>/g;
  let changed = false;

  const out = content.replace(re, (match, before, quote, cls, after, offset) => {
    // 计算缩进
    const lineStart = content.lastIndexOf('\n', offset) + 1;
    const indentMatch = /^[ \t]*/.exec(content.slice(lineStart, offset));
    const indent = indentMatch ? indentMatch[0] : '';

    const attrs = buildAttrsWithoutClass(before, after);
    const newClass = ensureInlineBlock(cls);

    const innerTag = `<UIcon${attrs} />`;
    const innerIndented = innerTag.replace(/\n/g, '\n' + indent + '  ');

    changed = true;
    return `${indent}<span class="${newClass}">\n${indent}  ${innerIndented}\n${indent}</span>`;
  });

  return { result: out, changed };
}

function transformNonSelfClosing(content) {
  // 非自闭合：<UIcon ... class="..." ...> ... </UIcon>
  const re = /<UIcon\b([\s\S]*?)\bclass\s*=\s*(["'])(.*?)\2([\s\S]*?)>([\s\S]*?)<\/UIcon>/g;
  let changed = false;

  const out = content.replace(re, (match, before, quote, cls, after, inner, offset) => {
    // 计算缩进
    const lineStart = content.lastIndexOf('\n', offset) + 1;
    const indentMatch = /^[ \t]*/.exec(content.slice(lineStart, offset));
    const indent = indentMatch ? indentMatch[0] : '';

    const attrs = buildAttrsWithoutClass(before, after);
    const newClass = ensureInlineBlock(cls);

    const innerBlock = `<UIcon${attrs}>${inner}</UIcon>`;
    const innerIndented = innerBlock.replace(/\n/g, '\n' + indent + '  ');

    changed = true;
    return `${indent}<span class="${newClass}">\n${indent}  ${innerIndented}\n${indent}</span>`;
  });

  return { result: out, changed };
}

function transformContent(content) {
  let anyChanged = false;

  // 先处理自闭合
  let { result: step1, changed: c1 } = transformSelfClosing(content);
  anyChanged = anyChanged || c1;

  // 再处理非自闭合（用 step1 作为输入）
  let { result: step2, changed: c2 } = transformNonSelfClosing(step1);
  anyChanged = anyChanged || c2;

  return { result: step2, changed: anyChanged };
}

function main() {
  const root = path.resolve(process.cwd(), 'app');
  if (!fs.existsSync(root)) {
    console.error('未找到 app 目录，脚本终止。');
    process.exit(1);
  }

  const files = walk(root).filter(f => f.endsWith('.vue'));
  const changedFiles = [];

  for (const file of files) {
    const raw = fs.readFileSync(file, 'utf8');
    const { result, changed } = transformContent(raw);
    if (changed && result !== raw) {
      fs.writeFileSync(file, result, 'utf8');
      changedFiles.push(file);
    }
  }

  if (changedFiles.length === 0) {
    console.log('未发现需要修改的 UIcon 用法（或已全部符合规范）。');
  } else {
    console.log(`已修改 ${changedFiles.length} 个文件：`);
    for (const f of changedFiles) console.log(' - ' + path.relative(process.cwd(), f));
  }
}

main();