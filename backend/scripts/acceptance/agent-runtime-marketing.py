#!/usr/bin/env python3
"""真实营销 SSE 验收：一次提交、断开后仅续订，检查权威 Trace 与唯一消息。"""
import argparse
import json
import os
from pathlib import Path
import sys
import time
from urllib.parse import urlparse
import uuid

import requests


def data(response):
    if response.status_code != 200:
        raise RuntimeError(f'HTTP 验收失败 status={response.status_code}，检查登录/授权/服务日志')
    value = response.json()
    if value.get('code') not in (None, 0, 200):
        raise RuntimeError('API 返回业务错误；检查服务日志')
    return value.get('data')


def events(response):
    event, seq, lines = 'message', None, []
    for line in response.iter_lines(chunk_size=1, decode_unicode=True):
        if line == '':
            if lines:
                yield event, seq, json.loads('\n'.join(lines))
            event, seq, lines = 'message', None, []
        elif line.startswith('event:'):
            event = line[6:].strip()
        elif line.startswith('id:'):
            seq = int(line[3:].strip())
        elif line.startswith('data:'):
            lines.append(line[5:].lstrip())
    # 不完整帧不作为已接收事件。


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--base-url', default='http://127.0.0.1:18077/api/v1')
    p.add_argument('--auth-file', required=True, help='仅从本机文件读取 access_token 或纯 token')
    p.add_argument('--tenant-uuid', required=True)
    p.add_argument('--input-file', required=True)
    p.add_argument('--report-file', required=True)
    p.add_argument('--team-key', default='marketing.campaign_review')
    p.add_argument('--read-timeout', type=int, default=1800)
    args = p.parse_args()
    url = urlparse(args.base_url)
    if url.hostname not in ('localhost', '127.0.0.1'):
        raise RuntimeError('此开发验收命令只发送到本机 Core')
    tenant = str(uuid.UUID(args.tenant_uuid))
    raw = Path(args.auth_file).read_text().strip()
    try:
        auth = json.loads(raw)
        token = auth.get('access_token') or (auth.get('data') or {}).get('access_token')
    except json.JSONDecodeError:
        token = raw
    if not token:
        raise RuntimeError('登录凭据文件缺少 access_token')
    client = requests.Session()
    client.headers.update(Authorization='Bearer ' + token.removeprefix('Bearer '),
                          **{'X-Tenant-UUID': tenant})
    base = args.base_url.rstrip('/')
    teams = data(client.get(base + '/admin/agents/teams', timeout=15))['items']
    selected = [t for t in teams if t.get('team_key') == args.team_key and t.get('status') == 'active']
    if len(selected) != 1:
        raise RuntimeError('当前租户需有唯一启用的营销团队；不自动修改团队配置')
    team = selected[0]
    session = data(client.post(base + '/agents/sessions', json={'env':'dev', 'agentId':team['parent_agent_id'],
                                'title':'Agent Runtime 持久化验收', 'singleton':False}, timeout=15))
    session_id = str(session['id'])
    message = Path(args.input_file).read_text().strip()
    if not message:
        raise RuntimeError('验收材料为空')
    submitted = dict(env='dev', agent_id=team['parent_agent_id'], session_id=session_id,
                     team_id=team['id'], parent_agent_id=team['parent_agent_id'],
                     client_msg_id=str(uuid.uuid4()), q=message)
    cursor, run_id, user_message_id = 0, None, None
    received_ids, timeline = set(), []
    begun = time.monotonic()
    response = client.get(base + '/agents/stream/sse', params=submitted, stream=True,
                          timeout=(15,args.read_timeout))
    response.encoding = 'utf-8'
    if response.status_code != 200:
        data(response)
    with response:
        for kind, seq, payload in events(response):
            if seq is not None:
                cursor = seq
                received_ids.add(seq)
            if kind == 'meta' and payload.get('durable_run') is True:
                run_id = str(uuid.UUID(payload['run_id']))
                user_message_id = str(payload['message_id'])
                break  # 主动断开。之后只传 run_id + after_seq，不再次提交用户输入。
            if kind in ('error', 'final', 'end'):
                raise RuntimeError('受理前终止；尚未建立持久化 Run')
    if not run_id:
        raise RuntimeError('没有收到 durable_run 受理确认')
    print(f'已受理 run={run_id}，主动断开后续订', flush=True)
    final = None
    for reconnect in range(3):
        response = client.get(base + '/agents/stream/sse', params={'env':'dev','run_id':run_id,'after_seq':str(cursor)},
                              stream=True, timeout=(15,args.read_timeout))
        response.encoding = 'utf-8'
        if response.status_code != 200:
            data(response)
        with response:
            for kind, seq, payload in events(response):
                if seq is not None:
                    if seq in received_ids or seq <= cursor:
                        raise RuntimeError('续订重复投递了已接收的事件序号')
                    cursor = seq
                    received_ids.add(seq)
                # 保存脱敏事件摘要；正文与凭据不写验收报告。
                inner = payload.get('payload') or payload
                timeline.append({'event':kind,'event_seq':seq,'task_id':inner.get('task_id'),
                                 'status':inner.get('status'),'reason_code':inner.get('reason_code')})
                if kind == 'final':
                    final = payload
                if kind == 'end':
                    break
        if final is not None:
            break
    if final is None:
        raise RuntimeError('原 Run 没有终态回复，保留 run_id 后排查')
    report = data(client.get(base + '/admin/agent-traces/messages/' + user_message_id + '/report',
                            params={'tenant_uuid':tenant,'session_id':session_id,'run_id':run_id,'format':'json'}, timeout=20))
    if report.get('summary',{}).get('state_backend') != 'redis':
        raise RuntimeError('追踪报告未使用 Redis 权威状态')
    messages = data(client.get(base + f'/agents/sessions/{session_id}/messages',params={'env':'dev','limit':200},timeout=15))['items']
    assistants = [m for m in messages if m.get('role') == 'assistant' and (m.get('meta') or {}).get('run_id') == run_id]
    users = [m for m in messages if m.get('role') == 'user']
    if len(assistants) != 1 or len(users) != 1:
        raise RuntimeError(f'消息去重失败 assistant={len(assistants)} user={len(users)}')
    envelope = (final.get('data') or {}).get('response_envelope')
    if final.get('success') is True and (not isinstance(envelope, dict) or envelope.get('schema') != 'powerx.agent.response/v4'):
        raise RuntimeError('营销终态缺少 v4 结构化证据报告')
    evidence = dict(run_id=run_id, session_id=session_id, message_id=user_message_id,
                    trace_id=report.get('trace_id'), state_backend='redis', event_seq=cursor,
                    status=report.get('summary',{}).get('status'), unique_assistant_messages=len(assistants),
                    user_messages=len(users), reconnected_without_resubmit=True,
                    response_schema=envelope.get('schema') if isinstance(envelope,dict) else None,
                    duration_seconds=round(time.monotonic()-begun,2),
                    events=timeline, tasks=report.get('run_state',{}).get('tasks',[]))
    dest = Path(args.report_file)
    dest.parent.mkdir(parents=True, exist_ok=True)
    fd = os.open(dest,os.O_CREAT|os.O_WRONLY|os.O_TRUNC,0o600)
    os.fchmod(fd,0o600)
    with os.fdopen(fd,'w') as stream:
        json.dump(evidence,stream,ensure_ascii=False,indent=2)
    if final.get('success') is not True or evidence['status'] != 'completed':
        raise RuntimeError(f'链路记录已保存，但业务未完成 status={evidence["status"]}')
    print(f'验收通过，脱敏证据={dest}',flush=True)


if __name__ == '__main__':
    try:
        main()
    except Exception as exc:
        print('验收失败：' + (str(exc) if isinstance(exc, RuntimeError) else type(exc).__name__), file=sys.stderr)
        sys.exit(1)
