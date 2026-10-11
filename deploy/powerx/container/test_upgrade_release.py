import copy
import hashlib
import json
from pathlib import Path
import unittest
import upgrade_release


class UpgradeReleaseTests(unittest.TestCase):
    def fixture(self):
        baseline=json.loads(upgrade_release.BASELINE.read_text())
        pair=copy.deepcopy(baseline);pair['source_sha']='a'*40;pair['release_id']='sha-'+pair['source_sha']
        for image in pair['images'].values():
            image['index_digest']='sha256:'+'b'*64
            for p in image['platforms'].values():p['digest']='sha256:'+'c'*64;p['source_revision']=pair['source_sha']
        report={'platform':'linux/amd64','status':'passed','source_sha':baseline['source_sha'],'target_sha':pair['source_sha'],
            'checks':{c:True for c in upgrade_release.REQUIRED_CHECKS},
            'source_images':{role:image['platforms']['linux/amd64']['digest'] for role,image in baseline['images'].items()},
            'target_images':{role:image['platforms']['linux/amd64']['digest'] for role,image in pair['images'].items()}}
        artifact={'report':report,'artifact':'upgrade-report-linux-amd64.json','sha256':hashlib.sha256(json.dumps(report).encode()).hexdigest()}
        return pair,baseline,[artifact]

    def manifest(self,pair,baseline,reports):
        return upgrade_release.build_manifest(pair,baseline,reports,'https://github.com/ArtisanCloud/PowerX/actions/runs/123','development')

    def test_manifest_distinguishes_built_and_verified_platforms(self):
        pair,baseline,reports=self.fixture();result=self.manifest(pair,baseline,reports)
        self.assertTrue(result['executable']);self.assertEqual(result['rollback']['mode'],'restore_required')
        self.assertEqual(result['verification']['tested_upgrade_platforms'],['linux/amd64'])
        self.assertIn('linux/arm64',result['images']['backend']['platforms'])
        self.assertFalse(result['configuration']['write_configuration'])
        try:import jsonschema
        except ImportError:self.skipTest('jsonschema not installed')
        schema=json.loads((upgrade_release.ROOT/'docs/contracts/powerx-upgrade-manifest.schema.json').read_text());jsonschema.validate(result,schema)

    def test_missing_failed_partial_wrong_pair_and_duplicate_evidence_rejected(self):
        for case in ['missing','failed','partial','wrong_source','wrong_image','wrong_revision','wrong_role','duplicate']:
            with self.subTest(case=case):
                pair,baseline,reports=self.fixture();report=reports[0]['report']
                if case=='missing':reports=[]
                if case=='failed':report['status']='failed'
                if case=='partial':report['checks']['interrupted_retry']=False
                if case=='wrong_source':report['source_sha']='d'*40
                if case=='wrong_image':report['target_images']['web_admin']='sha256:'+'f'*64
                if case=='wrong_revision':pair['images']['backend']['platforms']['linux/amd64']['source_revision']='d'*40
                if case=='wrong_role':pair['images']['backend']['repository']='ghcr.io/artisancloud/powerx-web-admin'
                if case=='duplicate':reports=reports*2
                with self.assertRaises(ValueError):self.manifest(pair,baseline,reports)

    def test_registry_fetch_does_not_accept_arbitrary_urls(self):
        with self.assertRaisesRegex(ValueError,'UPGRADE_IMAGE_REFERENCE_INVALID'):
            upgrade_release.resolve_image('backend','https://example.com/secret','a'*40)


if __name__=='__main__':unittest.main()
