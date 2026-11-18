#!/usr/bin/env python3
"""Summarise bench/results: python3 -I bench/report.py. One row per arm, model and task, p25/median/p75 per
metric, then one totals row per (model, arm): tasks passed and total cost, the cost-vs-correctness matrix."""
import glob, json, os, statistics

ROOT = os.path.dirname(os.path.abspath(__file__))
PIN_MODEL = json.load(open(os.path.join(ROOT, "protocol.json")))["pins"]["model"]


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
    groups.setdefault((r.get("arm") or "pinned", r.get("model") or PIN_MODEL, r["task"]), []).append(r)

print("arm model task ok/n  turns p25/med/p75  tool_bytes p25/med/p75  cost_usd p25/med/p75")
for (arm, model, task), rs in sorted(groups.items()):
    ok = sum(1 for r in rs if r["success"])
    cost = [round(r["cost_usd"], 4) if r["cost_usd"] is not None else None for r in rs]
    print(f"{arm} {model} {task} {ok}/{len(rs)}  {q([r['turns'] for r in rs])}  {q([r.get('tool_output_bytes') for r in rs])}  {q(cost)}")

totals = {}
for (arm, model, task), rs in groups.items():
    t = totals.setdefault((arm, model), [0, 0, 0.0])
    t[0] += sum(1 for r in rs if r["success"]); t[1] += len(rs)
    t[2] += sum(r["cost_usd"] or 0 for r in rs)
print("\ncost-vs-correctness (per model and arm): ok/n  total_cost_usd  cost_per_run")
for (arm, model), (ok, n, cost) in sorted(totals.items()):
    print(f"{arm} {model} {ok}/{n}  {cost:.3f}  {cost / n if n else 0:.4f}")
