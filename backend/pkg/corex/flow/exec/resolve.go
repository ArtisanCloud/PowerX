package exec

///******** 命名空间 ********/
//
//type NS struct {
//	Inputs      map[string]any            // 本步 inputs
//	Vars        map[string]any            // Flow.Variables + runtime params
//	TaskOutputs map[string]map[string]any // tid -> {"output": map[...]any}
//	History     any                       // 已按策略裁剪好的视图
//}
//
//func BuildNS(inputs, vars map[string]any, history any, taskOut map[string]map[string]any) *NS {
//	if inputs == nil {
//		inputs = map[string]any{}
//	}
//	if vars == nil {
//		vars = map[string]any{}
//	}
//	if taskOut == nil {
//		taskOut = map[string]map[string]any{}
//	}
//	return &NS{Inputs: inputs, Vars: vars, TaskOutputs: taskOut, History: history}
//}
//
///******** Binding 解析 ********/
//
//var (
//	reTask = regexp.MustCompile(`^\{\{\s*task\.([a-zA-Z0-9_-]+)\.output\.([a-zA-Z0-9_.-]+)\s*\}\}$`)
//	reVar  = regexp.MustCompile(`^\{\{\s*var\.([a-zA-Z0-9_.-]+)\s*\}\}$`)
//	reHist = regexp.MustCompile(`^\{\{\s*history\.([a-zA-Z0-9_\-\.\[\]]+)\s*\}\}$`)
//)
//
//func ResolveBinding(b Binding, ns *NS) (any, bool) {
//	// 1) 直接值
//	if b.Value != nil {
//		return b.Value, true
//	}
//	// 2) Ref
//	if s := strings.TrimSpace(b.Ref); s != "" {
//		// task
//		if mm := reTask.FindStringSubmatch(s); len(mm) == 3 {
//			tid, path := mm[1], mm[2]
//			out := ns.TaskOutputs[tid]
//			if out == nil {
//				return b.Default, false
//			}
//			v, ok := getByPath(out["output"], path)
//			if ok {
//				return v, true
//			}
//			return b.Default, false
//		}
//		// var
//		if mm := reVar.FindStringSubmatch(s); len(mm) == 2 {
//			if v, ok := getByPath(ns.Vars, mm[1]); ok {
//				return v, true
//			}
//			return b.Default, false
//		}
//		// history（先整体返回；若要解析 path，可把 History 规范为 []map 或 map 再走 getByPath）
//		if mm := reHist.FindStringSubmatch(s); len(mm) == 2 {
//			if ns.History == nil {
//				return b.Default, false
//			}
//			return ns.History, true
//		}
//	}
//	// 3) Expr（极简 evaluator，可日后换成 antonmedv/expr 或 JMESPath）
//	if e := strings.TrimSpace(b.Expr); e != "" {
//		if v, err := evalMini(e, ns); err == nil {
//			return v, true
//		}
//	}
//	// 4) Default
//	if b.Default != nil {
//		return b.Default, true
//	}
//	return nil, false
//}
//
///******** 辅助 ********/
//
//func getByPath(root any, path string) (any, bool) {
//	cur := root
//	for _, seg := range strings.Split(path, ".") {
//		mp, ok := cur.(map[string]any)
//		if !ok {
//			return nil, false
//		}
//		cur, ok = mp[seg]
//		if !ok {
//			return nil, false
//		}
//	}
//	if cur == nil {
//		return nil, false
//	}
//	return cur, true
//}
//
//// 极简表达式：concat(), sprintf(), 以及 JSON 字面量 + $.vars / $.inputs 的替换
//func evalMini(expr string, ns *NS) (any, error) {
//	s := strings.TrimSpace(expr)
//	// concat('a', $.vars.name, {{task.t1.output.id}})
//	if strings.HasPrefix(s, "concat(") && strings.HasSuffix(s, ")") {
//		args := splitArgs(s[len("concat(") : len(s)-1])
//		var sb strings.Builder
//		for _, a := range args {
//			v, _ := evalAtom(a, ns)
//			sb.WriteString(fmt.Sprint(v))
//		}
//		return sb.String(), nil
//	}
//	// sprintf('订单:%s', {{var.order_id}})
//	if strings.HasPrefix(s, "sprintf(") && strings.HasSuffix(s, ")") {
//		args := splitArgs(s[len("sprintf(") : len(s)-1])
//		if len(args) == 0 {
//			return "", nil
//		}
//		fmtStr, _ := evalAtom(args[0], ns)
//		vs := make([]any, 0, len(args)-1)
//		for _, a := range args[1:] {
//			v, _ := evalAtom(a, ns)
//			vs = append(vs, v)
//		}
//		return fmt.Sprintf(fmt.Sprint(fmtStr), vs...), nil
//	}
//	// JSON 对象/数组字面量（最简替换）
//	if (strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}")) ||
//		(strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]")) {
//		js := s
//		// 只替换一层：$.vars.k / $.inputs.k / $.history
//		for k, v := range ns.Vars {
//			js = strings.ReplaceAll(js, "$.vars."+k, utils.ToJSON(v))
//		}
//		for k, v := range ns.Inputs {
//			js = strings.ReplaceAll(js, "$.inputs."+k, utils.ToJSON(v))
//		}
//		if ns.History != nil {
//			js = strings.ReplaceAll(js, "$.history", utils.ToJSON(ns.History))
//		}
//		var out any
//		if err := json.Unmarshal([]byte(js), &out); err == nil {
//			return out, nil
//		}
//	}
//	// 兜底：原样返回
//	return s, nil
//}
//
//func evalAtom(a string, ns *NS) (any, bool) {
//	a = strings.TrimSpace(a)
//	if strings.HasPrefix(a, "'") && strings.HasSuffix(a, "'") {
//		return strings.Trim(a, "'"), true
//	}
//	if strings.HasPrefix(a, "$.vars.") {
//		return getByPath(ns.Vars, strings.TrimPrefix(a, "$.vars."))
//	}
//	if strings.HasPrefix(a, "$.inputs.") {
//		return getByPath(ns.Inputs, strings.TrimPrefix(a, "$.inputs."))
//	}
//	if a == "$.history" {
//		return ns.History, true
//	}
//	// 再试 Ref 语法
//	if v, ok := ResolveBinding(cx.Binding{Ref: a}, ns); ok {
//		return v, true
//	}
//	return a, true
//}
//
//func splitArgs(s string) []string {
//	raw := strings.Split(s, ",")
//	out := make([]string, 0, len(raw))
//	for _, r := range raw {
//		out = append(out, strings.TrimSpace(r))
//	}
//	return out
//}
