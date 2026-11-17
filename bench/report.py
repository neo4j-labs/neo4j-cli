#!/usr/bin/env python3
"""Summarise bench/results: python3 -I bench/report.py. One row per arm and task, p25/median/p75 per metric."""
import glob, json, os, statistics

ROOT = os.path.dirname(os.path.abspath(__file__))


def q(xs):
    xs = sorted(x for x in xs if x is not None)
    if not xs:
        return "-"
    if len(xs) == 1:
        return f"{xs[0]:g}"
    p25, med, p75 = statistics.quantiles(xs, n=4, method="inclusive")[0], statistics.median(xs), statistics.quantiles(xs, n=4, method="inclusive")[2]
    return f"{p25:g}/{med:g}/{p75:g}"


groups = {}
for f in sorted(glob.glob(os.path.join(ROOT, "results", "**", "result.json"), recursive=True)):
    r = json.load(open(f))
    groups.setdefault((r.get("arm") or "pinned", r["task"]), []).append(r)

print("arm task ok/n  turns p25/med/p75  tool_bytes p25/med/p75  cost_usd p25/med/p75")
for (arm, task), rs in sorted(groups.items()):
    ok = sum(1 for r in rs if r["success"])
    cost = [round(r["cost_usd"], 4) if r["cost_usd"] is not None else None for r in rs]
    print(f"{arm} {task} {ok}/{len(rs)}  {q([r['turns'] for r in rs])}  {q([r.get('tool_output_bytes') for r in rs])}  {q(cost)}")
