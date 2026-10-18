#!/usr/bin/env python3
"""离线校验 T199 Schema 和结构用例；不模拟生产授权、来源或 ledger 验真。"""

import copy
import argparse
import json
from pathlib import Path

from jsonschema import Draft202012Validator, FormatChecker


def patched(document, changes):
    """只实现夹具使用的 add/replace/remove JSON Pointer 操作。"""
    result = copy.deepcopy(document)
    for change in changes:
        parts = [part.replace("~1", "/").replace("~0", "~") for part in change["path"].split("/")[1:]]
        if not parts:
            raise ValueError("用例只能修改对象字段，不能替换根对象")
        parent = result
        for part in parts[:-1]:
            parent = parent[int(part)] if isinstance(parent, list) else parent[part]
        key = parts[-1]
        op = change["op"]
        if op not in {"add", "replace", "remove"}:
            raise ValueError(f"不支持的夹具操作：{op}")
        if isinstance(parent, list):
            index = len(parent) if key == "-" else int(key)
            if op == "remove":
                parent.pop(index)
            elif op == "add":
                parent.insert(index, copy.deepcopy(change["value"]))
            else:
                parent[index] = copy.deepcopy(change["value"])
        elif op == "remove":
            del parent[key]
        else:
            if op == "replace" and key not in parent:
                raise KeyError(key)
            parent[key] = copy.deepcopy(change["value"])
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--go-test-log", type=Path, help="校验 go test -v 输出的实际 DTO 样本；不代替 Go 语义断言")
    args = parser.parse_args()
    root = Path(__file__).resolve().parent
    schema = json.loads((root / "tool-evidence.v1.schema.json").read_text())
    cases = json.loads((root / "tool-evidence-cases.json").read_text())
    Draft202012Validator.check_schema(schema)
    formats = FormatChecker()
    if {"uuid", "date-time"} - formats.checkers.keys():
        raise RuntimeError("缺少 format 校验器；请使用合同文档中的 jsonschema[format] 安装命令")
    validator = Draft202012Validator(schema, format_checker=formats)
    seen = set()
    for group in ("schema_cases", "runtime_cases"):
        for case in cases[group]:
            if case["id"] in seen:
                raise ValueError(f"用例 ID 重复：{case['id']}")
            seen.add(case["id"])
            document = patched(cases["fixtures"][case["fixture"]], case["patch"])
            errors = list(validator.iter_errors(document))
            expected_valid = case["valid"] if group == "schema_cases" else True
            if bool(errors) == expected_valid:
                details = errors[0].message if errors else "未拒绝非法结构"
                raise AssertionError(f"{case['id']}: {details}")
            if group == "runtime_cases":
                expected = case["expected"]
                # 只检查语义用例的完整性；这里没有执行这些断言。
                required = {"stage", "code", "tool_calls", "task_state", "outcome", "failure_node"}
                if set(expected) != required or not isinstance(case["preconditions"], dict):
                    raise AssertionError(f"语义用例声明不完整：{case['id']}")
                if expected["failure_node"] != (expected["task_state"] == "failed"):
                    raise AssertionError(f"失败节点与状态不一致：{case['id']}")
    print(f"Schema 自检与 {len(cases['schema_cases'])} 个结构正反用例通过。")
    print(f"已检查 {len(cases['runtime_cases'])} 个语义用例的输入结构和声明完整性；未执行生产语义断言。")
    if args.go_test_log:
        count = 0
        for line in args.go_test_log.read_text().splitlines():
            marker = "tool_contract_fixture "
            if marker not in line:
                continue
            fixture = json.loads(line.split(marker, 1)[1])
            target = dict(schema, **{"$ref": f"#/$defs/{fixture['definition']}"})
            target.pop("oneOf", None)
            Draft202012Validator(target, format_checker=formats).validate(fixture["value"])
            count += 1
        if not count:
            raise AssertionError("Go 日志没有实际 DTO 样本；请运行文档中的 -v 命令")
        print(f"Go 实际执行输出的 {count} 个 wire DTO 与冻结 Schema 一致；执行语义以 Go 测试断言为准。")


if __name__ == "__main__":
    main()
