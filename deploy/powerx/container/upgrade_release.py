"""核对成对 OCI 镜像，并仅用已通过验收的报告生成升级执行制品。"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import sys
import urllib.request

ROOT = Path(__file__).resolve().parents[3]
BASELINE = Path(__file__).with_name('upgrade-baseline.json')
SHA = re.compile(r'^[a-f0-9]{40}$')
DIGEST = re.compile(r'^sha256:[a-f0-9]{64}$')
REPOSITORIES = {'backend': 'artisancloud/powerx-backend', 'web_admin': 'artisancloud/powerx-web-admin'}
REQUIRED_CHECKS = ['old_setup', 'configuration_preserved', 'login_preserved', 'business_data_preserved',
                   'files_preserved', 'plugin_runtime_preserved', 'new_schema', 'idempotent',
                   'unknown_source_rejected', 'invalid_config_rejected', 'legacy_blocker_rejected',
                   'plan_drift_rejected', 'partial_failure_retry', 'checksum_rejected',
                   'lock_conflict', 'interrupted_retry', 'binary_identity', 'paired_frontend', 'restore_rollback']


def resolve_image(role, reference, source_sha):
    if not SHA.fullmatch(source_sha) or not (DIGEST.fullmatch(reference) or reference == 'sha-' + source_sha):
        raise ValueError('UPGRADE_IMAGE_REFERENCE_INVALID')
    repo = REPOSITORIES[role]
    # 只访问固定 GHCR 仓库，不接受任意 registry URL、Bearer 或 DSN。
    request = urllib.request.Request('https://ghcr.io/token?service=ghcr.io&scope=repository:' + repo + ':pull')
    token = json.load(urllib.request.urlopen(request, timeout=30))['token']

    def fetch(kind, ref):
        request = urllib.request.Request('https://ghcr.io/v2/' + repo + '/' + kind + '/' + ref,
            headers={'Authorization': 'Bearer ' + token,
                     'Accept': 'application/vnd.oci.image.index.v1+json,application/vnd.oci.image.manifest.v1+json,application/vnd.docker.distribution.manifest.list.v2+json,application/vnd.docker.distribution.manifest.v2+json'})
        with urllib.request.urlopen(request, timeout=30) as response:
            raw = response.read()
        digest = 'sha256:' + hashlib.sha256(raw).hexdigest()
        if DIGEST.fullmatch(ref) and digest != ref:
            raise ValueError('UPGRADE_IMAGE_DIGEST_MISMATCH')
        return json.loads(raw), digest

    index, index_digest = fetch('manifests', reference)
    platforms = {}
    for desc in index.get('manifests', []):
        platform = desc.get('platform', {})
        key = platform.get('os', '') + '/' + platform.get('architecture', '')
        if key not in ('linux/amd64', 'linux/arm64'):
            continue  # OCI attestations 不当成可执行平台。
        image, _ = fetch('manifests', desc['digest'])
        config, _ = fetch('blobs', image['config']['digest'])
        revision = config.get('config', {}).get('Labels', {}).get('org.opencontainers.image.revision')
        if revision != source_sha:
            raise ValueError('UPGRADE_IMAGE_REVISION_MISMATCH')
        platforms[key] = {'digest': desc['digest'], 'source_revision': revision}
    if not platforms:
        raise ValueError('UPGRADE_IMAGE_PLATFORMS_MISSING')
    return {'repository': 'ghcr.io/' + repo, 'index_digest': index_digest, 'platforms': platforms}


def build_manifest(pair, baseline, reports, ci_run_url, channel):
    source = pair['source_sha']
    if not SHA.fullmatch(source) or pair['release_id'] != 'sha-' + source:
        raise ValueError('UPGRADE_RELEASE_ID_INVALID')
    if channel not in ('development', 'stable'):
        raise ValueError('UPGRADE_CHANNEL_INVALID')
    for role, repository in REPOSITORIES.items():
        image=pair['images'][role]
        if image['repository']!='ghcr.io/'+repository or not DIGEST.fullmatch(image['index_digest']):
            raise ValueError('UPGRADE_IMAGE_REFERENCE_INVALID')
        for platform in image['platforms'].values():
            if platform['source_revision']!=source or not DIGEST.fullmatch(platform['digest']):
                raise ValueError('UPGRADE_IMAGE_REVISION_MISMATCH')
    if not re.fullmatch(r'https://github.com/ArtisanCloud/PowerX/actions/runs/[0-9]+', ci_run_url):
        raise ValueError('UPGRADE_CI_EVIDENCE_INVALID')
    verified = []
    evidence = []
    for artifact in reports:
        report=artifact['report']
        platform = report['platform']
        if report.get('status') != 'passed' or not all(report.get('checks', {}).get(c) is True for c in REQUIRED_CHECKS):
            raise ValueError('UPGRADE_ACCEPTANCE_INCOMPLETE')
        if report.get('source_sha') != baseline['source_sha'] or report.get('target_sha') != source:
            raise ValueError('UPGRADE_EVIDENCE_SOURCE_MISMATCH')
        for role in REPOSITORIES:
            if report['source_images'][role] != baseline['images'][role]['platforms'][platform]['digest'] or report['target_images'][role] != pair['images'][role]['platforms'][platform]['digest']:
                raise ValueError('UPGRADE_EVIDENCE_IMAGE_MISMATCH')
        if platform in verified:
            raise ValueError('UPGRADE_DUPLICATE_PLATFORM_EVIDENCE')
        verified.append(platform)
        if not re.fullmatch(r'upgrade-report-linux-(amd64|arm64)\.json',artifact['artifact']) or not re.fullmatch(r'[a-f0-9]{64}',artifact['sha256']):
            raise ValueError('UPGRADE_EVIDENCE_ARTIFACT_INVALID')
        evidence.append({'platform': platform, 'artifact': artifact['artifact'], 'sha256': artifact['sha256']})
    if not verified:
        raise ValueError('UPGRADE_ACCEPTANCE_MISSING')
    payloads = json.loads((ROOT/'backend/cmd/database/upgrade_payloads.json').read_text())
    return {'schema_version': 'powerx.upgrade-manifest/v1', 'release_id': pair['release_id'],
            'channel': channel, 'version': ('dev-' if channel == 'development' else 'release-') + source[:12],
            'source_sha': source, 'executable': True, 'images': pair['images'],
            'supported_from': [{'release_id': baseline['release_id'], 'source_sha': baseline['source_sha'], 'images': baseline['images']}],
            'database': {'migration_version': hashlib.sha256((ROOT/'backend/cmd/database/upgrade_payloads.json').read_bytes()).hexdigest(),
                         'payloads': payloads, 'postgres_major': [16], 'required_extensions': ['vector'],
                         'schema_changes': True, 'data_backfill': True, 'system_catalog': True,
                         'knowledge_binding_in_plan': True},
            'configuration': {'changes': [], 'preserve_installed': True, 'write_configuration': False, 'missing_configuration': 'block'},
            'compatibility': {'host_bridge': 2, 'session_action': 'relogin_if_legacy_access_token',
                              'validated_scope': {'storage_drivers':['local'],'vector_topologies':['same_core_database','disabled'],'plugins':[{'id':'com.powerx.upgrade.fixture','version':'1.0.0'}],'other_bindings':'require_additional_acceptance'},
                              'plugin_validation': 'fixture_runtime_only; validate each installed plugin against declared identity/Host contracts',
                              'post_upgrade_actions': ['Review new service permissions and explicitly grant required API Key/Profile permissions; existing grants and tenant version locks are preserved',
                                                       'Media file preservation is verified through the protected Admin JWT resource endpoint; legacy /media UUID presign links are not a validated compatibility surface. Align clients with the current Admin resource or service-actor Host transfer contract.']},
            'execution': {'mode': 'offline', 'stop_all_writers': True, 'strict_configuration': True, 'lock_timeout_seconds': 30,
                          'nontransactional_steps': ['knowledge'], 'recovery': 'restore_matching_pre_upgrade_backup'},
            'rollback': {'mode': 'restore_required', 'release_id': baseline['release_id'], 'images': baseline['images'],
                         'restore': ['database', 'configuration', 'runtime_files', 'external_vector_database_if_configured'],
                         'new_writes_after_upgrade': 'must be exported or will be lost when restoring the pre-upgrade backup'},
            'verification': {'status': 'passed', 'ci_run_url': ci_run_url, 'tested_upgrade_platforms': sorted(verified),
                             'reports': evidence}}


def main():
    parser = argparse.ArgumentParser()
    commands = parser.add_subparsers(dest='command', required=True)
    resolve = commands.add_parser('resolve')
    resolve.add_argument('--source-sha', required=True)
    resolve.add_argument('--backend', required=True)
    resolve.add_argument('--web-admin', required=True)
    resolve.add_argument('--output', required=True)
    publish = commands.add_parser('manifest')
    publish.add_argument('--pair', required=True)
    publish.add_argument('--reports-dir', required=True)
    publish.add_argument('--ci-run-url', required=True)
    publish.add_argument('--channel', choices=['development', 'stable'], required=True)
    publish.add_argument('--output', required=True)
    args = parser.parse_args()
    if args.command == 'resolve':
        result = {'release_id': 'sha-' + args.source_sha, 'source_sha': args.source_sha,
                  'images': {role: resolve_image(role, args.backend if role == 'backend' else args.web_admin, args.source_sha) for role in REPOSITORIES}}
    else:
        reports = [{'report':json.loads(p.read_text()),'artifact':p.name,'sha256':hashlib.sha256(p.read_bytes()).hexdigest()} for p in sorted(Path(args.reports_dir).rglob('upgrade-report-*.json'))]
        result = build_manifest(json.loads(Path(args.pair).read_text()), json.loads(BASELINE.read_text()), reports, args.ci_run_url, args.channel)
    Path(args.output).write_text(json.dumps(result, indent=2) + '\n')


if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        code = str(error) if isinstance(error, ValueError) and str(error).startswith('UPGRADE_') else 'UPGRADE_RELEASE_METADATA_FAILED'
        print(code, file=sys.stderr)
        sys.exit(1)
