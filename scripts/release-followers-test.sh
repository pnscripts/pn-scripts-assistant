#!/usr/bin/env bash
#
# The workflows that follow the Release workflow act only on a real release.
#
# release.yml also runs on the button, from any branch, to rehearse a release
# without publishing it. On 10.11.2026 the rehearsal of 1.0.1 from
# fix/1.0.1-only-me-withdrawn finished green, and its workflow_run started
# "A stranger installs it" (which took the branch name for a tag: "release
# not found"), "Apt repository" and "winget" against a release that had not
# changed. Only a Release run started by a tag push is a release, so every job
# that a workflow_run can start first asks for that.
#
# And their checks can fail. Under `bash -e` a line `! cmd` never stops the
# step (bash leaves `!` out of errexit), so "! ldd ... | grep 'not found'"
# passed whatever ldd said. Each such line ends in `|| exit 1`.
#
# Run by `make check`.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

python3 - "$ROOT/.github/workflows" <<'PY'
import pathlib, sys, yaml

wanted = "github.event.workflow_run.event == 'push'"
bad, followers = [], []
for path in sorted(pathlib.Path(sys.argv[1]).glob("*.yml")):
    doc = yaml.safe_load(path.read_text())
    triggers = doc.get(True, doc.get("on")) or {}  # YAML 1.1 reads `on` as true
    if not isinstance(triggers, dict) or "workflow_run" not in triggers:
        continue
    if "Release" not in (triggers["workflow_run"] or {}).get("workflows", []):
        continue
    followers.append(path.name)
    for name, job in (doc.get("jobs") or {}).items():
        if job.get("needs"):
            continue  # skipped with the job it needs
        cond = str(job.get("if", ""))
        if wanted not in cond:
            bad.append(f"{path.name}: job {name} runs after a Release rehearsal (its if: lacks {wanted!r})")

if not followers:
    sys.exit("✗ no workflow follows Release: has the release moved? Update this test.")
for path in sorted(pathlib.Path(sys.argv[1]).glob("*.yml")):
    for n, line in enumerate(path.read_text().splitlines(), 1):
        s = line.strip()
        if s.startswith("! ") and not s.endswith("|| exit 1"):
            bad.append(f"{path.name}:{n}: '{s}' cannot fail the step; end it with || exit 1")

for line in bad:
    print("✗", line, file=sys.stderr)
if bad:
    sys.exit(1)
print("✓ after Release, only a tag push starts:", ", ".join(followers))
PY
