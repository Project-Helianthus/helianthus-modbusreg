#!/usr/bin/env python3
"""Choose package tests, compare, preflight and the existing full suite.

Stack position comes from open pull-request base and head repository ids and
refs. A parent is the open request whose head is the child's base. Manifest
selection uses the recorded row and the sealed destination paths.
"""
from __future__ import annotations

import argparse
import json
from pathlib import Path, PurePosixPath
import sys

_SHORT_NAMES = {
    "gateway": "helianthus-gateway",
    "helianthus-ebusgateway": "helianthus-gateway",
    "semreg": "helianthus-semreg",
    "ebusreg": "helianthus-eebusreg",
    "ebusgo": "helianthus-ebusgo",
    "modbusreg": "helianthus-modbusreg",
    "modbus": "helianthus-modbus",
    "canbusreg": "helianthus-canbusreg",
    "canbus": "helianthus-canbus",
    "eebusreg": "helianthus-eebusreg",
    "eebus-go": "helianthus-eebus-go",
    "matterreg": "helianthus-matterreg",
    "helianthus-matter-binding": "helianthus-matterreg",
    "ha-integration": "helianthus-ha-integration",
    "vrc-explorer": "helianthus-vrc-explorer",
    "helianthus-vrc-explorer": "helianthus-vrc-explorer",
    "docs-semantic": "helianthus-docs-semantic",
    "addon": "helianthus-ha-addon",
    "helianthus-ha-addon": "helianthus-ha-addon",
}
PUBLIC_NAMES = dict(_SHORT_NAMES)
for _public in _SHORT_NAMES.values():
    PUBLIC_NAMES.setdefault(_public, _public)

DESTINATION_IDS = frozenset({"modbusreg", "helianthus-modbusreg"})
FULL_SUITE = "./scripts/ci_local.sh"
_FULL = {
    "run_full": True,
    "run_packages": False,
    "run_compare": False,
    "run_preflight": False,
    "run_gates": False,
}


def public_url(name):
    public = PUBLIC_NAMES.get(name)
    if not public:
        raise KeyError(name)
    return "https://github.com/Project-Helianthus/" + public + ".git"


def parse_name_status(text):
    changes = []
    for line in text.splitlines():
        if not line.strip():
            continue
        parts = line.split("\t")
        code = parts[0][:1]
        if code in {"R", "C"} and len(parts) >= 3:
            changes.append((code, parts[1], parts[2]))
        elif len(parts) >= 2:
            changes.append((code, parts[1], None))
    return changes


def change_paths(changes):
    paths = []
    for _status, path, renamed in changes:
        paths.append(path)
        if renamed:
            paths.append(renamed)
    return paths


def additions_only(changes):
    if not changes:
        return False
    return all(status in {"A", "C"} for status, _path, _renamed in changes)


def packages(paths):
    found = set()
    for path in paths:
        if not str(path).endswith(".go"):
            continue
        parent = PurePosixPath(path).parent.as_posix()
        if parent in {"", "."}:
            found.add(".")
        else:
            found.add("./" + parent)
    return sorted(found)


def sealed_destination_paths(manifest, destination_ids):
    paths = []
    for entry in manifest.get("paths") or []:
        if not isinstance(entry, dict) or entry.get("repository") not in destination_ids:
            continue
        for path in entry.get("paths") or []:
            if isinstance(path, str):
                paths.append(path)
    return paths


def covers(manifest, changed, destination_ids):
    row = manifest.get("row")
    if not isinstance(row, str) or not row.strip() or not changed:
        return False
    sealed = set(sealed_destination_paths(manifest, destination_ids))
    if not sealed:
        return False
    return all(path in sealed for path in changed)


def load_manifests(directory):
    found = []
    if directory is None or not Path(directory).is_dir():
        return found
    for path in sorted(Path(directory).glob("*.json")):
        try:
            data = json.loads(path.read_text(encoding="utf-8"))
        except (OSError, json.JSONDecodeError):
            continue
        if isinstance(data, dict):
            found.append((path, data))
    return found


def identify(manifests, changed, destination_ids=DESTINATION_IDS):
    matched = []
    for path, manifest in manifests:
        if covers(manifest, changed, destination_ids):
            matched.append((path, manifest))
    if not matched:
        return "none", None, None
    if len(matched) > 1:
        return "many", None, None
    return "one", matched[0][0], matched[0][1]


def _side(pull, name):
    side = pull.get(name) or {}
    repo = side.get("repo") or {}
    ref = side.get("ref")
    repo_id = repo.get("id")
    if not isinstance(ref, str) or not ref or repo_id is None:
        return None
    return ref, repo_id


def _relations(current_number, current_base, current_head, pulls):
    parents = 0
    children = 0
    for other in pulls:
        if not isinstance(other, dict):
            return None
        if other.get("number") == current_number:
            continue
        base = _side(other, "base")
        head = _side(other, "head")
        if base is None or head is None:
            return None
        if current_base == head:
            parents += 1
        if base == current_head:
            children += 1
    return parents, children


def stack_position(current, pulls):
    if not isinstance(current, dict) or not isinstance(pulls, list):
        return "unresolved"
    current_base = _side(current, "base")
    current_head = _side(current, "head")
    if current_base is None or current_head is None or "number" not in current:
        return "unresolved"
    related = _relations(current.get("number"), current_base, current_head, pulls)
    if related is None or related[0] > 1:
        return "unresolved"
    if related[1]:
        return "lower"
    if related[0] == 1:
        return "tip"
    return "unstacked"


def dispatch(identified, position, additive):
    if identified != "one" or position == "unresolved":
        return dict(_FULL)
    if position == "lower":
        return {
            "run_full": False,
            "run_packages": True,
            "run_compare": True,
            "run_preflight": additive,
            "run_gates": additive,
        }
    if position == "tip":
        return {
            "run_full": True,
            "run_packages": False,
            "run_compare": additive,
            "run_preflight": additive,
            "run_gates": False,
        }
    if position == "unstacked" and additive:
        return {
            "run_full": False,
            "run_packages": True,
            "run_compare": True,
            "run_preflight": True,
            "run_gates": True,
        }
    return dict(_FULL)


def branch_name(identified, position, additive):
    if identified == "none":
        return "no-manifest"
    if identified == "many" or position == "unresolved":
        return "unresolved"
    if position == "lower" and additive:
        return "lower-additive"
    if position == "lower":
        return "lower-nonadditive"
    if position == "tip" and additive:
        return "tip-additive"
    if position == "tip":
        return "tip-nonadditive"
    if additive:
        return "unstacked-additive"
    return "unstacked-nonadditive"


def build_plan(changes, position, identified, manifest_path=None, manifest=None):
    additive = (
        identified == "one"
        and position in {"lower", "tip", "unstacked"}
        and additions_only(changes)
    )
    flags = dispatch(identified, position, additive)
    plan = {
        "selected": identified == "one",
        "branch": branch_name(identified, position, additive),
        "position": position if identified != "many" else "unresolved",
        "additive": additive,
        "manifest": "" if manifest_path is None else str(manifest_path),
        "row": "",
        "packages": packages(change_paths(changes)),
        "acquisition": [],
    }
    if isinstance(manifest, dict) and isinstance(manifest.get("row"), str):
        plan["row"] = manifest["row"]
    plan.update(flags)
    return plan


def after_check(plan, check_code):
    updated = dict(plan)
    updated["exit_code"] = 0
    if plan.get("selected") and check_code != 0:
        updated["branch"] = "selected-check-failed"
        updated["run_packages"] = False
        updated["run_compare"] = False
        updated["run_preflight"] = False
        updated["run_gates"] = False
        updated["exit_code"] = check_code
    return updated


def _reference_pair(record):
    if not isinstance(record, dict):
        return None
    repository = record.get("repository")
    revision = record.get("revision")
    if not repository or not revision:
        return None
    return public_url(repository), revision


def exemplar_fetches(manifest):
    found = []
    for pattern in manifest.get("patterns") or []:
        if not isinstance(pattern, dict):
            continue
        pair = _reference_pair(pattern.get("exemplar") or {})
        if pair:
            found.append(pair)
    return found


def _candidate_fetch(pattern, found):
    candidate = pattern.get("candidate") or {}
    if not isinstance(candidate, dict) or candidate.get("form") != "reference":
        return
    pair = _reference_pair(candidate.get("reference") or candidate)
    if pair:
        found.append(pair)


def reference_fetches(manifest):
    found = []
    for pattern in manifest.get("patterns") or []:
        if isinstance(pattern, dict):
            _candidate_fetch(pattern, found)
    for expectation in manifest.get("expectations") or []:
        if not isinstance(expectation, dict):
            continue
        for reference in expectation.get("references") or []:
            pair = _reference_pair(reference)
            if pair:
                found.append(pair)
    for difference in manifest.get("expected_differences") or []:
        if not isinstance(difference, dict):
            continue
        pair = _reference_pair(difference.get("reference") or {})
        if pair:
            found.append(pair)
    return found


def acquisition_lines(manifest):
    donor = manifest.get("donor") or {}
    lines = [
        "donor-url " + public_url(donor["repository"]),
        "donor-sha " + str(donor["sha"]),
    ]
    for url, revision in exemplar_fetches(manifest):
        lines.append("donor-fetch " + url + " " + revision)
    for url, revision in reference_fetches(manifest):
        lines.append("tool-fetch " + url + " " + revision)
    return lines


def _next_link(header):
    if not header:
        return None
    for part in header.split(","):
        section = part.strip()
        if 'rel="next"' not in section:
            continue
        start = section.find("<")
        end = section.find(">")
        if start >= 0 and end > start:
            return section[start + 1:end]
    return None


def _request_headers(token):
    headers = {
        "Accept": "application/vnd.github+json",
        "User-Agent": "modbusreg-preflight",
    }
    if token:
        headers["Authorization"] = "Bearer " + token
    return headers


class _PageRequest:
    def __init__(self, url, headers):
        self.full_url = url
        self.headers = headers

    def has_header(self, name):
        return name in self.headers


def _read_page(opener, url, token):
    request = _PageRequest(url, _request_headers(token))
    with opener(request) as response:
        payload = json.loads(response.read().decode())
        link = ""
        headers = getattr(response, "headers", {})
        if headers:
            link = headers.get("Link", "")
        return payload, _next_link(link)


def fetch_open_pulls(repository, opener=None, token=None, first_url=None):
    if opener is None:
        return None
    url = first_url or (
        "https://api.github.com/repos/" + repository + "/pulls?state=open&per_page=100"
    )
    pulls = []
    while url:
        try:
            payload, url = _read_page(opener, url, token)
        except Exception:
            if token:
                return fetch_open_pulls(
                    repository, opener=opener, token=None, first_url=first_url
                )
            return None
        if not isinstance(payload, list):
            return None
        pulls.extend(payload)
    return pulls


def retained_plan():
    plan = {
        "selected": False,
        "branch": "retained",
        "position": "retained",
        "additive": False,
        "manifest": "",
        "row": "",
        "packages": [],
        "acquisition": [],
    }
    plan.update(_FULL)
    return plan


def _optional_int(value):
    if value is None or value == "":
        return None
    return int(value)


def current_pull(args):
    if args.current_number is None:
        return None
    return {
        "number": args.current_number,
        "base": {"ref": args.base_ref, "repo": {"id": _optional_int(args.base_repo_id)}},
        "head": {"ref": args.head_ref, "repo": {"id": _optional_int(args.head_repo_id)}},
    }


def resolve_pulls(args):
    if args.pulls_file:
        data = json.loads(Path(args.pulls_file).read_text(encoding="utf-8"))
        return data if isinstance(data, list) else None
    return None


def classify_invocation(args):
    if args.event != "pull_request":
        return retained_plan()
    changes = []
    if args.name_status:
        changes = parse_name_status(Path(args.name_status).read_text(encoding="utf-8"))
    manifests = load_manifests(args.manifests_dir)
    status, path, manifest = identify(manifests, change_paths(changes))
    pulls = resolve_pulls(args)
    position = "unresolved" if pulls is None else stack_position(current_pull(args), pulls)
    plan = build_plan(changes, position, status, path, manifest)
    if plan["selected"]:
        plan["acquisition"] = acquisition_lines(manifest)
    return plan


def _text(value):
    if value is True:
        return "true"
    if value is False:
        return "false"
    return "" if value is None else str(value)


def write_github_output(path, plan):
    if not path:
        return
    keys = (
        "selected",
        "branch",
        "position",
        "additive",
        "run_full",
        "run_packages",
        "run_compare",
        "run_preflight",
        "run_gates",
        "manifest",
        "row",
    )
    with open(path, "a", encoding="utf-8") as handle:
        for key in keys:
            handle.write(key + "=" + _text(plan.get(key)) + "\n")


def write_side_files(args, plan):
    if args.acquisition:
        text = "\n".join(plan.get("acquisition") or [])
        Path(args.acquisition).write_text(text + ("\n" if text else ""), encoding="utf-8")
    if args.packages_file:
        Path(args.packages_file).write_text(
            "\n".join(plan.get("packages") or []) + "\n",
            encoding="utf-8",
        )
    if args.selected_file:
        Path(args.selected_file).write_text(
            "true\n" if plan.get("selected") else "false\n",
            encoding="utf-8",
        )


def parse_args(argv):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--event", default="pull_request")
    parser.add_argument("--manifests-dir")
    parser.add_argument("--name-status")
    parser.add_argument("--pulls-file")
    parser.add_argument("--current-number", type=int)
    parser.add_argument("--base-ref", default="")
    parser.add_argument("--head-ref", default="")
    parser.add_argument("--base-repo-id", default="")
    parser.add_argument("--head-repo-id", default="")
    parser.add_argument("--github-output")
    parser.add_argument("--acquisition")
    parser.add_argument("--packages-file")
    parser.add_argument("--selected-file")
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    try:
        plan = classify_invocation(args)
    except KeyError as exc:
        print("unmapped repository " + str(exc), file=sys.stderr)
        return 1
    write_github_output(args.github_output, plan)
    write_side_files(args, plan)
    return 0


if __name__ == "__main__":
    sys.exit(main())
