#!/usr/bin/env python3
"""Trigger evaluation that targets the *real* installed neo4j-cli skill.

We rewrite the description line in the user's installed
~/.claude/skills/neo4j-cli/SKILL.md, run claude -p for each prompt,
watch the stream for `Skill(skill: "neo4j-cli")` (or a Read of the
skill file), then restore the original SKILL.md.

This is more faithful than registering a fake slash-command because
`claude -p` actually surfaces the installed skill in the model's
available_skills list, while project-scoped slash commands and
project-scoped SKILL.md files do not appear there.
"""
from __future__ import annotations

import argparse
import atexit
import json
import os
import re
import select
import shutil
import signal
import subprocess
import sys
import time
from concurrent.futures import ThreadPoolExecutor, as_completed
from pathlib import Path

SKILL_PATH = Path.home() / ".claude" / "skills" / "neo4j-cli" / "SKILL.md"
BACKUP_PATH = SKILL_PATH.with_suffix(".md.autoresearch_backup")
SKILL_NAME = "neo4j-cli"

# Worker-process state — initialized inside the worker so module-level
# subprocess.Popen never inherits the parent's signal handlers.


def patch_description(skill_md: str, new_desc: str) -> str:
    """Replace just the `description:` frontmatter line. Description is
    expected to be single-line for our skill (the renderer emits it that
    way). New description is also collapsed to a single line."""
    one_line = " ".join(new_desc.strip().splitlines())
    out_lines = []
    in_fm = False
    fm_seen = 0
    replaced = False
    for line in skill_md.splitlines(keepends=True):
        stripped = line.strip()
        if stripped == "---":
            fm_seen += 1
            in_fm = fm_seen == 1
            out_lines.append(line)
            continue
        if in_fm and not replaced and re.match(r"^description:\s*", line):
            out_lines.append(f"description: {one_line}\n")
            replaced = True
            continue
        out_lines.append(line)
    if not replaced:
        raise RuntimeError("Could not find description: line in SKILL.md frontmatter")
    return "".join(out_lines)


def restore() -> None:
    if BACKUP_PATH.exists():
        shutil.move(str(BACKUP_PATH), str(SKILL_PATH))


def install_signal_handlers() -> None:
    atexit.register(restore)
    for sig in (signal.SIGINT, signal.SIGTERM):
        signal.signal(sig, lambda *_: (restore(), os._exit(130)))


def run_single_query(query: str, timeout: int, model: str | None) -> bool:
    """Spawn `claude -p`, collect all output, scan assistant tool_use events
    for invocation of the neo4j-cli skill (or Read of the skill file)."""
    cmd = [
        "claude",
        "-p", query,
        "--output-format", "stream-json",
        "--verbose",
        "--include-partial-messages",
    ]
    if model:
        cmd.extend(["--model", model])
    env = {k: v for k, v in os.environ.items() if k != "CLAUDECODE"}
    process = subprocess.Popen(
        cmd,
        stdout=subprocess.PIPE,
        stderr=subprocess.DEVNULL,
        stdin=subprocess.DEVNULL,
        env=env,
    )
    skill_path_marker = "skills/neo4j-cli/SKILL.md"
    debug = os.environ.get("EVAL_DEBUG") == "1"
    triggered = False
    decided = False  # set once we observe the FIRST tool_use (Skill triggers; anything else does not)
    try:
        out, _ = process.communicate(timeout=timeout)
    except subprocess.TimeoutExpired:
        process.kill()
        process.wait()
        return False
    for line in out.splitlines():
        if not line.strip():
            continue
        try:
            ev = json.loads(line)
        except json.JSONDecodeError:
            continue
        if ev.get("type") != "assistant":
            continue
        for c in ev.get("message", {}).get("content", []):
            if c.get("type") != "tool_use":
                continue
            name = c.get("name", "")
            inp = c.get("input", {}) or {}
            if debug:
                print(f"[dbg] tool_use name={name} input={json.dumps(inp)[:120]}", file=sys.stderr, flush=True)
            if name == "Skill" and inp.get("skill") == SKILL_NAME:
                triggered = True
                decided = True
                break
            if name == "Read" and skill_path_marker in str(inp.get("file_path", "")):
                triggered = True
                decided = True
                break
            # Any other tool means the model bypassed the skill
            decided = True
            break
        if decided:
            break
    return triggered


def run_eval(eval_set: list[dict], num_workers: int, timeout: int,
             runs_per_query: int, trigger_threshold: float,
             model: str | None) -> dict:
    triggers_by_q: dict[str, list[bool]] = {}
    items_by_q: dict[str, dict] = {}
    with ThreadPoolExecutor(max_workers=num_workers) as exe:
        fut_to_q: dict = {}
        for item in eval_set:
            for _ in range(runs_per_query):
                fut = exe.submit(run_single_query, item["query"], timeout, model)
                fut_to_q[fut] = item
        for fut in as_completed(fut_to_q):
            item = fut_to_q[fut]
            q = item["query"]
            items_by_q[q] = item
            triggers_by_q.setdefault(q, [])
            try:
                triggers_by_q[q].append(fut.result())
            except Exception as e:
                print(f"warn: query failed: {e}", file=sys.stderr)
                triggers_by_q[q].append(False)
    results = []
    for q, trs in triggers_by_q.items():
        item = items_by_q[q]
        rate = sum(trs) / len(trs)
        passed = (rate >= trigger_threshold) if item["should_trigger"] else (rate < trigger_threshold)
        results.append({
            "query": q,
            "should_trigger": item["should_trigger"],
            "trigger_rate": rate,
            "triggers": sum(trs),
            "runs": len(trs),
            "pass": passed,
        })
    return {"results": results, "summary": {
        "total": len(results),
        "passed": sum(1 for r in results if r["pass"]),
        "failed": sum(1 for r in results if not r["pass"]),
    }}


def main() -> None:
    p = argparse.ArgumentParser()
    p.add_argument("--eval-set", required=True)
    p.add_argument("--description-file", required=True,
                   help="Path to the candidate description.txt to splice into SKILL.md")
    p.add_argument("--num-workers", type=int, default=8)
    p.add_argument("--runs-per-query", type=int, default=2)
    p.add_argument("--timeout", type=int, default=90)
    p.add_argument("--trigger-threshold", type=float, default=0.5)
    p.add_argument("--model", default=None)
    p.add_argument("--output", default=None,
                   help="Write JSON output to this file (atomic). If omitted, stdout.")
    p.add_argument("--verbose", action="store_true")
    args = p.parse_args()

    if not SKILL_PATH.exists():
        print(f"error: real skill not installed at {SKILL_PATH}", file=sys.stderr)
        print("run `neo4j-cli skill install` first", file=sys.stderr)
        sys.exit(2)

    eval_set = json.loads(Path(args.eval_set).read_text())
    new_desc = Path(args.description_file).read_text()

    # Backup + patch
    if BACKUP_PATH.exists():
        # leftover from a prior interrupted run — restore first then re-back-up
        shutil.move(str(BACKUP_PATH), str(SKILL_PATH))
    shutil.copy(str(SKILL_PATH), str(BACKUP_PATH))
    install_signal_handlers()
    try:
        original = SKILL_PATH.read_text()
        patched = patch_description(original, new_desc)
        SKILL_PATH.write_text(patched)
        out = run_eval(
            eval_set=eval_set,
            num_workers=args.num_workers,
            timeout=args.timeout,
            runs_per_query=args.runs_per_query,
            trigger_threshold=args.trigger_threshold,
            model=args.model,
        )
    finally:
        restore()

    if args.verbose:
        s = out["summary"]
        print(f"results: {s['passed']}/{s['total']}", file=sys.stderr)
        for r in out["results"]:
            tag = "PASS" if r["pass"] else "FAIL"
            print(f"  [{tag}] rate={r['triggers']}/{r['runs']} expected={r['should_trigger']}: {r['query'][:80]}", file=sys.stderr)
    payload = json.dumps(out, indent=2)
    if args.output:
        tmp = Path(args.output + ".tmp")
        tmp.write_text(payload)
        tmp.replace(Path(args.output))
    else:
        print(payload)


if __name__ == "__main__":
    main()
