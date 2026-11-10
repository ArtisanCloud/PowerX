"""只向明确配置的个人 open_id 发送飞书运维消息；凭据和 token 不落日志。"""
import json
import os
from pathlib import Path
import stat
import urllib.request
import uuid

CONFIG = Path('/etc/powerx-transition-guard/feishu-personal.json')


def request_json(url, payload, token=None):
    headers = {'Content-Type': 'application/json; charset=utf-8'}
    if token:
        headers['Authorization'] = 'Bearer ' + token
    request = urllib.request.Request(url, data=json.dumps(payload).encode(), headers=headers)
    with urllib.request.urlopen(request, timeout=5) as response:
        result = json.load(response)
    if result.get('code') != 0:
        raise RuntimeError('feishu_api_rejected_code_' + str(result.get('code')))
    return result


def send_personal(text, event_id, path=CONFIG):
    if not path.exists():
        return {'status': 'not_configured'}
    info = path.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_uid != 0 or info.st_mode & 0o077:
        raise RuntimeError('notification_config_must_be_root_private_regular_file')
    with os.fdopen(os.open(path, os.O_RDONLY | os.O_NOFOLLOW)) as stream:
        config = json.load(stream)
    recipient = config.get('receive_id', '')
    if (config.get('mode') != 'feishu_app' or config.get('receive_id_type') != 'open_id'
            or not recipient.startswith('ou_') or not config.get('app_id', '').startswith('cli_')
            or not config.get('app_secret')):
        raise RuntimeError('personal_feishu_configuration_invalid')
    access = request_json('https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal',
                          {'app_id': config['app_id'], 'app_secret': config['app_secret']})
    response = request_json('https://open.feishu.cn/open-apis/im/v1/messages?receive_id_type=open_id',
                            {'receive_id': recipient, 'msg_type': 'text',
                             'content': json.dumps({'text': text}, ensure_ascii=False),
                             'uuid': str(uuid.uuid5(uuid.NAMESPACE_URL, event_id))},
                            access['tenant_access_token'])
    message_id = response.get('data', {}).get('message_id')
    if not message_id:
        raise RuntimeError('feishu_message_acceptance_unconfirmed')
    return {'status': 'api_accepted', 'message_id': message_id, 'recipient_read_not_verified': True}
