# 日期、数字与货币本地化规则

> 统一 PowerX Web Admin 中日期、数字、货币的格式规则，确保多语言环境下显示一致。默认时区参考产品要求为 **Asia/Tokyo**，可根据租户配置调整。

---

## 1. 时间与日期

| 场景 | 格式 | 示例 (Asia/Tokyo) |
| --- | --- | --- |
| 日期 | `YYYY-MM-DD` | `2025-02-14` |
| 日期时间 | `YYYY-MM-DD HH:mm` | `2025-02-14 09:30` |
| 含秒 | `YYYY-MM-DD HH:mm:ss` | `2025-02-14 09:30:45` |
| 长格式（界面标题） | `YYYY年M月D日 dddd` or locale variant | `2025年2月14日 金曜日` |
| 相对时间 | `x 分钟前` | `5 分钟前` |

### 实现建议

- 统一使用 `dayjs` + `dayjs/plugin/utc` + `dayjs/plugin/timezone`。  
- 创建工具 `formatDate(value, format = "YYYY-MM-DD")`，内部自动应用默认时区：  
  ```ts
  import dayjs from "dayjs";
  import utc from "dayjs/plugin/utc";
  import timezone from "dayjs/plugin/timezone";

  dayjs.extend(utc);
  dayjs.extend(timezone);
  dayjs.tz.setDefault("Asia/Tokyo");
  ```
- 若租户可自定义时区，可在用户上下文中注入 `tenant.timezone` 并覆盖 `dayjs.tz.setDefault()`。

---

## 2. 数字与千分位

- 使用 `Intl.NumberFormat(locale, { maximumFractionDigits: 2 })`。  
- 在 i18n 中提供 Helper：  
  ```ts
  export const formatNumber = (value: number, options?: Intl.NumberFormatOptions) =>
    new Intl.NumberFormat(locale.value, { maximumFractionDigits: 2, ...options }).format(value);
  ```
- 对于摘要卡片（`2.4M`）可使用缩写工具（`Intl.NumberFormat` + `notation: "compact"`）。

---

## 3. 货币

- 默认货币：JPY（可按租户设置 USD/CNY 等）。  
- 使用 `Intl.NumberFormat(locale, { style: "currency", currency: tenantCurrency })`。  
- 对 Token 消耗等单位，明确是否使用货币符号或自定义单位（如 “2.4M Tokens”）。

---

## 4. 文本资源

- 国际化字符串中若包含数字/日期占位，使用 `{value}` 模式：  
  ```json
  {
    "dashboard": {
      "lastUpdated": "数据更新于 {time}"
    }
  }
  ```
- 渲染时传入 `t("dashboard.lastUpdated", { time: formatDateTime(lastUpdated) })`。

---

## 5. 测试与验证

- QA 在每种语言下检查数字/日期是否遵循本地习惯。  
- 使用自动化测试验证不同租户时区的显示（例如切换为 `UTC`, `America/New_York`）。  
- 对前端输入（如筛选表单）在提交前转换为 ISO 字符串（`toISOString()`），避免时区偏移。

---

## 6. Review Checklist

- [ ] 新组件是否统一使用 `formatDate`、`formatCurrency` 等工具。  
- [ ] 显示与输入时区是否一致。  
- [ ] 货币/数字格式是否根据 locale 自动切换。  
- [ ] 文案是否避免硬编码日期/数字格式。  
- [ ] 单位/符号是否清晰（例如“万円” vs “JPY”）。

---

## 7. 后续计划

- 引入 `@nuxtjs/i18n` 的 datetime/number 格式插件或自定义指令。  
- 在用户设置中允许选择日期格式/时区/货币单位。  
- 将格式化工具集中放在 `app/utils/formatters.ts`，统一使用。  
- 为导出/报表提供标准化格式（CSV 日期、数字小数点等）。
