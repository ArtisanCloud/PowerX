// --- 类型定义（可复用在任何树） ---
export type AnyRecord = Record<string, any>;

export interface TreeItemBase {
  id: string;
  title: string;
  icon?: string;
  path?: string;
  order?: number;
  visible?: boolean;
  permissions?: string[];
  parentId?: string;
  children?: TreeItemBase[];
  // 透传其他字段
  [key: string]: any;
}

export interface AdapterOptions<TOut extends TreeItemBase = TreeItemBase> {
  /**
   * 数组所在位置的路径（点/数组混合均可）
   * 例："data.menus" | ["data", "menus"] | ""(若 resp 本身就是数组)
   */
  arrayPath?: string | Array<string | number>;

  /**
   * children 字段名（默认 "children"）
   */
  childrenKey?: string;

  /**
   * 字段映射：把原始节点映射成目标节点（可只映射你关心的字段，其他透传）
   * - 入参 node：原始节点对象
   * - 返回：目标节点（必须至少包含 id / title，其他随意）
   */
  mapNode?: (node: AnyRecord) => Partial<TOut> & Pick<TOut, "id" | "title">;

  /**
   * 排序函数（默认按 order 升序，然后 title，再 id）
   */
  sortFn?: (a: TOut, b: TOut) => number;

  /**
   * 过滤函数（默认不过滤；可用于权限/可见性）
   * - 在 map + children 递归完成后调用
   */
  filterFn?: (node: TOut) => boolean;

  /**
   * 是否稳定排序（默认 true）
   */
  stableSort?: boolean;
}

/** 工具：取深路径 */
function getByPath(obj: any, path?: AdapterOptions["arrayPath"]) {
  if (!path || path === "") return obj;
  const segs = Array.isArray(path) ? path : String(path).split(".");
  let cur = obj;
  for (const k of segs) {
    if (cur == null) return undefined;
    cur = cur[k as any];
  }
  return cur;
}

/** 工具：稳健断言数组 */
function asArray(value: any): any[] {
  if (Array.isArray(value)) return value;
  return [];
}

/** 默认节点映射（通用约定） */
function defaultMapNode<TOut extends TreeItemBase>(n: AnyRecord): TOut {
  const node = n ?? {};
  const out: TreeItemBase = {
    id: String(node.id ?? node.key ?? node.code ?? ""),
    title: String(node.title ?? node.name ?? ""),
    icon: typeof node.icon === "string" ? node.icon : undefined,
    path: typeof node.path === "string" ? node.path : undefined,
    order: Number.isFinite(node.order) ? Number(node.order) : 0,
    visible: typeof node.visible === "boolean" ? node.visible : true,
    permissions: Array.isArray(node.permissions) ? node.permissions : undefined,
    parentId: typeof node.parentId === "string" ? node.parentId : undefined,
    // 先不塞 children，递归时再填
  };
  // 透传其它字段
  return Object.assign({}, node, out) as TOut;
}

/** 默认排序：order -> title -> id */
function defaultSort(a: TreeItemBase, b: TreeItemBase) {
  const ao = a.order ?? 0,
    bo = b.order ?? 0;
  if (ao !== bo) return ao - bo;
  if (a.title !== b.title)
    return String(a.title).localeCompare(String(b.title));
  return String(a.id).localeCompare(String(b.id));
}

/**
 * 通用树适配器：从任意响应中抽取树形数组并规范化
 */
export function adaptTreeArray<TOut extends TreeItemBase = TreeItemBase>(
  resp: any,
  options?: AdapterOptions<TOut>
): TOut[] {
  const {
    arrayPath = "data.menus",
    childrenKey = "children",
    mapNode = defaultMapNode as AdapterOptions<TOut>["mapNode"],
    sortFn = defaultSort as AdapterOptions<TOut>["sortFn"],
    filterFn,
    stableSort = true,
  } = options || {};

  const rawArray = getByPath(resp, arrayPath);
  const input = asArray(rawArray);

  // 稳定排序需要保留原始索引（若你要绝对稳定）
  const withIndex = input.map((n, idx) => ({ __idx: idx, n }));

  const walk = (list: AnyRecord[]): TOut[] => {
    const out = list
      .filter((x) => x && typeof x === "object")
      .map((x) => {
        const mapped = mapNode!(x);
        // 递归 children
        const rawChildren = asArray(x[childrenKey]);
        const children = rawChildren.length ? walk(rawChildren) : [];
        return { ...x, ...mapped, children } as unknown as TOut;
      });

    // 过滤（可见性/权限）
    const filtered = filterFn ? out.filter(filterFn) : out;

    // 排序
    if (stableSort) {
      // 稳定排序：在 sort 相等时保留原序（通过附加索引）
      return filtered
        .map((x, i) => ({ x, i }))
        .sort((A, B) => {
          const r = sortFn!(A.x, B.x);
          return r !== 0 ? r : A.i - B.i;
        })
        .map((z) => z.x);
    }
    return filtered.sort(sortFn);
  };

  return walk(withIndex.map((v) => v.n));
}
