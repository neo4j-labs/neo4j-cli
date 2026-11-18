#!/usr/bin/env python3
"""LLM-as-judge scorer over stored bench runs: python3 -I bench/score.py [--force] [--task T1] .
Judges a compacted view of each run (commands, output sizes, final answer) with the model from
bench/scorers.json and writes score.json next to result.json. Skips runs that already have one
(unless --force) and runs the sampling check in scorers.json; a run below the sampling rate is
recorded as not judged, never silently dropped. The deterministic checks in run.py stay the
source of truth for success; these grades cover only what code cannot check."""
import argparse, glob, json, os, random, statistics, subprocess

ROOT = os.path.dirname(os.path.abspath(__file__))
CFG = json.load(open(os.path.join(ROOT, "scorers.json")))
PIN_MODEL = json.load(open(os.path.join(ROOT, "protocol.json")))["pins"]["model"]
ap = argparse.ArgumentParser()
ap.add_argument("--force", action="store_true", help="re-judge runs that already have a score.json")
ap.add_argument("--task", help="judge only this task id")
ARGS = ap.parse_args()


def compact(path):
    """stream.jsonl -> list of (command, output_bytes) plus the assistant text turns."""
    cmds, texts = [], []
    pending = {}
    for line in open(path):
        try:
            e = json.loads(line)
        except ValueError:
            continue
        if e.get("type") == "assistant":
            for c in e["message"].get("content", []):
                if c.get("type") == "tool_use" and c.get("name") == "Bash":
                    pending[c.get("id")] = c["input"].get("command", "")
                if c.get("type") == "text" and c.get("text", "").strip():
                    texts.append(c["text"].strip())
        if e.get("type") == "user":
            for c in (e.get("message", {}).get("content") or []):
                if isinstance(c, dict) and c.get("type") == "tool_result" and c.get("tool_use_id") in pending:
                    t = c.get("content")
                    cmds.append((pending.pop(c["tool_use_id"]), len((t if isinstance(t, str) else json.dumps(t)).encode())))
    return cmds, texts


def judge(rec, cmds, texts):
    trace = "\n".join(f"$ {c}   -> {n} bytes of output" for c, n in cmds)
    asks = "\n".join(f'- "{k}" (0-10, pass at or below {v["max"]}): {v["ask"]}' for k, v in CFG["scorers"].items())
    prompt = (
        "You grade one run of a CLI agent benchmark. The agent could only run neo4j-cli and a few read-only "
        "commands. Below are the shell commands it ran (with output sizes), its other messages, and its final "
        f"answer to the task: {rec['task']} — {rec['final'][:600]}\n\n"
        f"Tool trace:\n{trace}\n\nAgent messages:\n" + "\n".join(f"* {t[:200]}" for t in texts[:10]) +
        f"\n\nGrade each scorer on its 0-10 scale and reply with ONLY this JSON object, nothing else:\n"
        '{"' + '": 0, "'.join(CFG["scorers"]) + '": 0, "notes": "one short sentence"}\n'
        "Scores: lower is better.\n" + asks
    )
    r = subprocess.run(["claude", "-p", prompt, "--model", CFG["judge_model"], "--max-budget-usd", str(CFG["max_budget_usd"])],
                       capture_output=True, text=True, timeout=300)
    out = r.stdout.strip()
    try:
        obj = json.loads(out[out.index("{"):out.rindex("}") + 1])
    except ValueError:
        return {"judged": False, "reason": f"unparseable judge reply: {out[:200]}"}
    scores, passes = {}, {}
    for k, v in CFG["scorers"].items():
        s = obj.get(k)
        if not isinstance(s, (int, float)):
            return {"judged": False, "reason": f"score {k} missing or not a number: {obj}"}
        scores[k] = s
        passes[k] = s <= v["max"]
    return {"judged": True, "judge_model": CFG["judge_model"], "scores": scores, "pass": passes, "notes": obj.get("notes", "")}


summary = {}
for f in sorted(glob.glob(os.path.join(ROOT, "results", "**", "result.json"), recursive=True)):
    rec = json.load(open(f))
    if ARGS.task and rec["task"] != ARGS.task:
        continue
    d = os.path.dirname(f)
    out = os.path.join(d, "score.json")
    if os.path.exists(out) and not ARGS.force:
        s = json.load(open(out))
    elif random.random() > CFG["sampling_rate"]:
        s = {"judged": False, "reason": "below sampling rate"}
        json.dump(s, open(out, "w"), indent=1)
    else:
        cmds, texts = compact(os.path.join(d, "stream.jsonl"))
        s = judge(rec, cmds, texts)
        json.dump(s, open(out, "w"), indent=1)
    key = (rec.get("arm") or "pinned", rec.get("model") or PIN_MODEL, rec["task"])
    if s.get("judged"):
        summary.setdefault(key, []).append(s)

print("arm model task  " + "  ".join(f"{k} (med, pass x/n)" for k in CFG["scorers"]))
for (arm, model, task), ss in sorted(summary.items()):
    cells = []
    for k in CFG["scorers"]:
        xs = [s["scores"][k] for s in ss]
        cells.append(f"{statistics.median(xs):g} {sum(1 for s in ss if s['pass'][k])}/{len(ss)}")
    print(f"{arm} {model} {task}  " + "  ".join(cells))
