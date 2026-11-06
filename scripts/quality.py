#!/usr/bin/env python3
"""Local complexity snapshots; metric warnings never fail the command."""

import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[1]
OUTPUT = ROOT / ".quality"
METRICS = {"gocognit": "cognitive", "gocyclo": "cyclomatic", "funlen": "long_lines"}


def collect():
    version = subprocess.check_output(["golangci-lint", "version"], text=True).strip()
    with tempfile.TemporaryDirectory(prefix="opencode-quality-") as directory:
        report = Path(directory) / "lint.json"
        subprocess.run(
            ["golangci-lint", "run", "--config", "scripts/quality.yml",
             "--path-mode", "abs", "--issues-exit-code", "0",
             "--output.text.path", os.devnull, "--output.json.path", str(report), "./..."],
            cwd=ROOT, check=True, stdout=subprocess.DEVNULL,
        )
        issues = json.loads(report.read_text()).get("Issues") or []
    return {"schema": 1, "tool": version,
            "revision": subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=ROOT, text=True).strip(),
            "config": (ROOT / "scripts/quality.yml").read_text(),
            "functions": parse_issues(issues)}


def parse_issues(issues):
    functions = {}
    for issue in issues:
        metric = METRICS.get(issue["FromLinter"])
        if metric is None:
            raise ValueError(f"Unexpected diagnostic: {issue['Text']}")
        position = issue["Pos"]
        path = str(Path(position["Filename"]).relative_to(ROOT))
        # Source declarations disambiguate methods with the same name.
        declaration = issue["SourceLines"][0].strip()
        match = re.match(r"func (?:\(([^)]+)\) )?(\w+)\s*\(", declaration)
        if match is None:
            raise ValueError(f"Cannot identify function: {declaration}")
        receiver, name = match.groups()
        if receiver:
            name = f"({receiver.split()[-1]}).{name}"
        key = f"{path}:{name}"
        row = functions.setdefault(key, {"file": path, "function": name, "line": position["Line"]})
        pattern = r"too long \((\d+) >" if metric == "long_lines" else r"complexity (\d+) of func"
        value = re.search(pattern, issue["Text"])
        if value is None:
            raise ValueError(f"Cannot read metric: {issue['Text']}")
        row[metric] = int(value.group(1))
    return dict(sorted(functions.items()))


def is_test(row):
    return row["file"].endswith("_test.go")


def hotspots(row):
    limits = {"cognitive": 10 if is_test(row) else 15, "cyclomatic": 10}
    if is_test(row):
        limits["long_lines"] = 60
    return [f"{metric}={row[metric]}>{limit}" for metric, limit in limits.items()
            if row.get(metric, 0) > limit]


def render(current, baseline=None):
    lines = ["# Local quality report", "", "Warnings guide review; they are not gates.", "",
             "| Group | Functions | Cognitive total | Cognitive max | Cyclomatic max | Hotspots |",
             "|---|---:|---:|---:|---:|---:|"]
    for test, label in [(False, "Production"), (True, "Tests and test helpers")]:
        rows = [r for r in current["functions"].values() if is_test(r) == test]
        lines.append(f"| {label} | {len(rows)} | {sum(r.get('cognitive', 0) for r in rows)} | "
                     f"{max((r.get('cognitive', 0) for r in rows), default=0)} | "
                     f"{max((r.get('cyclomatic', 0) for r in rows), default=0)} | "
                     f"{sum(bool(hotspots(r)) for r in rows)} |")
    lines += ["", "Totals include newly extracted helpers: inspect them alongside maximum scores.", "",
              "## Hotspots", ""]
    for row in sorted(current["functions"].values(), key=lambda r: (-r.get("cognitive", 0), r["file"], r["line"])):
        warnings = hotspots(row)
        if warnings:
            lines.append(f"- {row['file']}:{row['line']} `{row['function']}`: {', '.join(warnings)}")
    if baseline:
        lines += ["", "## Changes from baseline", "",
                  "| Function | Cognitive | Cyclomatic | Lines when >60 |",
                  "|---|---:|---:|---:|"]
        before, after = baseline["functions"], current["functions"]
        for key in sorted(before.keys() | after.keys()):
            old, new = before.get(key, {}), after.get(key, {})
            metrics = ("cognitive", "cyclomatic", "long_lines")
            if key in before and key in after and all(old.get(m) == new.get(m) for m in metrics):
                continue
            changes = [f"{old.get(m, '—')} → {new.get(m, '—')}" for m in metrics]
            lines.append(f"| `{key}` | {' | '.join(changes)} |")
        lines += ["", "New/deleted/renamed functions appear separately; line moves alone are ignored."]
    lines += ["", "Length counts follow funlen (comments ignored); only lengths above 60 are collected.",
              "Readability needs a separate evidence-based review: docs/development/quality-review.md."]
    return "\n".join(lines) + "\n"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", nargs="?", choices=["report", "baseline", "compare"], default="report")
    args = parser.parse_args()
    baseline = None
    baseline_path = OUTPUT / "baseline.json"
    if args.action == "compare":
        if not baseline_path.exists():
            parser.error("No baseline. Run mise run quality baseline before refactoring.")
        baseline = json.loads(baseline_path.read_text())
    current = collect()
    if baseline and any(current[k] != baseline.get(k) for k in ("schema", "tool", "config")):
        parser.error("Tool or configuration changed. Capture a fresh baseline before comparing.")
    OUTPUT.mkdir(exist_ok=True)
    if args.action == "baseline":
        baseline_path.write_text(json.dumps(current, indent=2) + "\n")
        print(f"Baseline saved: {baseline_path}")
    (OUTPUT / "current.json").write_text(json.dumps(current, indent=2) + "\n")
    report = render(current, baseline)
    (OUTPUT / "report.md").write_text(report)
    print(report)


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, subprocess.CalledProcessError) as error:
        print(f"quality: {error}", file=sys.stderr)
        sys.exit(1)
