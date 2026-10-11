"""构建前生成/核对镜像内执行载荷的内容寻址校验和。"""
import argparse
import hashlib
import json
from pathlib import Path
import sys

ROOT = Path(__file__).resolve().parents[3]
OUTPUT = ROOT / 'backend/cmd/database/upgrade_payloads.json'
COMMON = ['backend/cmd/database/upgrade_steps.go', 'backend/cmd/database/migrate.go', 'backend/go.mod', 'backend/go.sum']
GROUPS = {
    'core': ['backend/pkg/corex/db/database/**/*.go', 'backend/pkg/corex/db/migration/**/*.go',
             'backend/pkg/corex/db/persistence/model/**/*.go', 'backend/pkg/dynamic_form/persistence/model/*.go'],
    'agent': ['backend/internal/server/agent/persistence/**/*.go'],
    'knowledge': ['backend/pkg/corex/db/migration/*knowledge*.go', 'backend/pkg/corex/db/persistence/vectorstore/**/*.go'],
    'catalog': ['backend/config/platform_capabilities/*.yaml',
                'backend/internal/service/integration_gateway/base_capabil*.go',
                'backend/internal/service/integration_gateway/apikeypermissions/*.go',
                'backend/pkg/corex/db/persistence/repository/capability_registry/*.go',
                'backend/pkg/corex/db/persistence/repository/iam/permission*.go'],
}


def payloads():
    result = []
    for name, patterns in GROUPS.items():
        files = set(ROOT / p for p in COMMON)
        for pattern in patterns:
            files.update(ROOT.glob(pattern))
        entries = []
        for path in sorted(files):
            if path.is_file() and not path.name.endswith('_test.go'):
                entries.append([str(path.relative_to(ROOT)), hashlib.sha256(path.read_bytes()).hexdigest()])
        digest = hashlib.sha256(json.dumps(entries, separators=(',', ':')).encode()).hexdigest()
        result.append({'name': name, 'checksum': digest})
    return result


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--check', action='store_true')
    args = parser.parse_args()
    content = json.dumps(payloads(), indent=2) + '\n'
    if args.check:
        if not OUTPUT.is_file() or OUTPUT.read_text() != content:
            print('UPGRADE_PAYLOAD_CHANGED: regenerate upgrade_payloads.json and review the new migration IDs', file=sys.stderr)
            return 1
    else:
        OUTPUT.write_text(content)
    return 0


if __name__ == '__main__':
    sys.exit(main())
