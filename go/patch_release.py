import io
p = '.github/workflows/release.yml'
s = open(p).read()
trig_old = '''on:
  push:
    tags:
      - "v*.*.*"
'''
trig_new = '''on:
  push:
    tags:
      - "v*.*.*"
  # Manual fallback: build a release for any existing tag (e.g. when a tag
  # push event was missed). Run from the Actions tab, passing the tag name.
  workflow_dispatch:
    inputs:
      tag:
        description: "要发布的标签（如 v1.1.0）"
        required: true
        type: string
'''
assert trig_old in s, 'trig not found'
s = s.replace(trig_old, trig_new, 1)
job_old = '''jobs:
  release:
    # Skip pre-release tags (e.g. v0.1.2-beta, v0.1.2-rc1).
    # Only clean version tags trigger a release.
    if: "!contains(github.ref, '-')"
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4
        with:
          fetch-depth: 0
'''
D = chr(36)  # dollar
job_new = '''jobs:
  release:
    # Skip pre-release tags (e.g. v0.1.2-beta, v0.1.2-rc1).
    # Only clean version tags trigger a release.
    if: !<>
    runs-on: ubuntu-latest
    env:
      RELEASE_TAG: @<>
    steps:
      - name: Checkout
        uses: actions/checkout@v4
        with:
          fetch-depth: 0
          ref: =<>
'''
job_new = job_new.replace('!<>', D + '{{ !contains(github.event_name == ' + chr(39) + 'workflow_dispatch' + chr(39) + ' && inputs.tag || github.ref_name, ' + chr(39) + '-' + chr(39) + ') }}')
job_new = job_new.replace('@<>', D + '{{ github.event_name == ' + chr(39) + 'workflow_dispatch' + chr(39) + ' && inputs.tag || github.ref_name }}')
job_new = job_new.replace('=<>', D + '{{ env.RELEASE_TAG }}')
assert job_old in s, 'job not found'
s = s.replace(job_old, job_new, 1)
ver_old = '        run: echo ' + chr(34) + 'VERSION=' + D + '{GITHUB_REF#refs/tags/}' + chr(34) + ' >> ' + D + 'GITHUB_ENV'
ver_new = '        run: echo ' + chr(34) + 'VERSION=' + D + '{{ env.RELEASE_TAG }}' + chr(34) + ' >> ' + D + 'GITHUB_ENV'
assert ver_old in s, 'ver not found'
s = s.replace(ver_old, ver_new, 1)
open(p, 'w').write(s)
print('patched release.yml')
