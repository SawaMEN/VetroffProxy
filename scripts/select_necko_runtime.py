#!/usr/bin/env python3
"""Select a compiled Actions artifact for this exact native source lock.

Wait for a matching source build rather than running changed native code against
an older binary. Compiled artifacts from a failed test run remain useful: all
runtime hashes/provenance and native gates are checked again by the consumer.
"""
import argparse
import base64
import json
import os
from pathlib import Path
import subprocess
import time


def matching_run(runs, expected, source_for_sha, artifacts_for_run, arch='amd64'):
    if arch not in ('amd64', 'arm64', 'both'):
        raise ValueError('unsupported runtime architecture')
    required = set()
    if arch in ('amd64', 'both'):
        required.update(('necko-linux-amd64-runtime', 'necko-source-provenance'))
    if arch in ('arm64', 'both'):
        required.update(('necko-linux-arm64-runtime', 'necko-source-provenance-arm64'))
    waiting = False
    for run in runs:
        if source_for_sha(run['head_sha']) != expected:
            continue
        artifacts = artifacts_for_run(run['id'])
        names = {a['name'] for a in artifacts if not a.get('expired')}
        if required <= names:
            return run['id'], waiting
        if run['status'] != 'completed':
            waiting = True
    return None, waiting


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--arch', choices=('amd64', 'arm64', 'both'), default='amd64')
    parser.add_argument('--repo', default=os.environ.get('GITHUB_REPOSITORY'))
    parser.add_argument('--lock', type=Path, default=Path('experiments/firefox-necko/gecko.lock.json'))
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--wait-seconds', type=int, default=10800)
    parser.add_argument('--allow-missing', action='store_true')
    args = parser.parse_args()
    if not args.repo or len(args.repo.split('/')) != 2:
        parser.error('repository must be owner/name')
    expected = json.loads(args.lock.read_text())
    cache = {}

    def api(path):
        return json.loads(subprocess.check_output(['gh', 'api', 'repos/' + args.repo + '/' + path],
                                                  text=True, timeout=60))

    def source(sha):
        if sha not in cache:
            try:
                content = api('contents/experiments/firefox-necko/gecko.lock.json?ref=' + sha)
                cache[sha] = json.loads(base64.b64decode(content['content']))
            except (subprocess.CalledProcessError, ValueError, KeyError):
                cache[sha] = None
        return cache[sha]

    deadline = time.monotonic() + args.wait_seconds
    # Push workflows may become visible in different orders. Give the source
    # workflow time to register before deciding no matching build exists.
    discovery_deadline = min(deadline, time.monotonic() + 120)
    while True:
        # Release workflows can compile the runtime through workflow_call too.
        runs = []
        for workflow in ('necko-experiment.yml', 'release.yml'):
            runs.extend(api(f'actions/workflows/{workflow}/runs?per_page=100')['workflow_runs'])
        runs.extend(api('actions/workflows/necko-runtime-test.yml/runs?per_page=30&branch=main&status=completed')['workflow_runs'])
        runs.sort(key=lambda run: run['id'], reverse=True)
        run, waiting = matching_run(runs, expected, source,
                                   lambda run_id: api(f'actions/runs/{run_id}/artifacts')['artifacts'], args.arch)
        expired = time.monotonic() >= deadline or (not waiting and time.monotonic() >= discovery_deadline)
        if run is not None or (expired and args.allow_missing):
            with args.output.open('a') as output:
                output.write(f'run_id={run or 0}\n')
            if run is None:
                print('No matching complete Gecko build; cold source build required')
            else:
                print('Selected compiled runtime run', run)
            return
        if expired:
            raise SystemExit('No compiled runtime matches the source lock; inspect the source build.')
        print('Waiting for compilation of the matching native source lock', flush=True)
        time.sleep(30)


if __name__ == '__main__':
    main()
