#!/usr/bin/env python3
"""neo4j-cli agent benchmark runner: python3 -I bench/run.py <task> <run-number>. Writes bench/results/<task>-<n>/."""
import hashlib, json, os, re, shutil, subprocess, sys, time, tempfile

ROOT = os.path.dirname(os.path.abspath(__file__))
P = json.load(open(os.path.join(ROOT, "protocol.json")))
BIN = P["pins"]["neo4j_cli"]["path"]
MODEL = P["pins"]["model"]
H = P["harness"]

PROMPTS = {
    "T1": "Using the neo4j-cli command, find out which stored database (DBMS) credentials exist and tell me the name of the stored credential. Use the CLI itself to discover how, not the internet.",
    "T2": "Using neo4j-cli, set the global default output format to json and confirm the change took effect.",
    "T3": "Run `neo4j-cli --format nope config list`. It will fail. Fix the command using only the information in the error message, and run it successfully. Tell me the command that worked.",
    "T4": "Install the neo4j-cli agent skill for Claude Code only (not other agents) and tell me which skill files were installed.",
    "T5": "Using neo4j-cli's own discovery output (not source code or the internet), list the `aura instance` subcommands that change state, so they need --rw. Reply with the subcommand names only.",
    "T6": "Show the 5 most recent neo4j-cli command history entries, newest first, as a JSON array of the command strings only. Reply with the JSON array and nothing else.",
    "T7": "Using neo4j-cli, start a throwaway local Neo4j in Docker (community edition), run the Cypher `RETURN 1+41 AS answer` against it, and tell me the answer. Remove the container when done.",
}


def cli(home, *args):
    env = dict(os.environ, HOME=home)
    return subprocess.run([BIN, *args], env=env, capture_output=True, text=True)


def setup(task, home):
    os.makedirs(home, exist_ok=True)
    cli(home, "config", "set", "credential-storage", "insecure", "--rw")
    if task == "T1":
        cli(home, "credential", "dbms", "add", "--name", "bench-db", "--uri", "neo4j://localhost:7687",
            "--username", "neo4j", "--password", "bench-pass", "--rw")
    if task == "T4":
        os.makedirs(os.path.join(home, ".claude"), exist_ok=True)
    if task == "T6":
        for a in (["config", "list"], ["config", "get", "format"], ["credential", "dbms", "list"],
                  ["skill", "list"], ["agent-context"], ["history", "list"], ["config", "list"]):
            cli(home, *a)


def managed_containers():
    r = subprocess.run(["docker", "ps", "-aq", "--filter", "label=org.neo4j.cli.managed=true"], capture_output=True, text=True)
    return r.stdout.split()


def parse_stream(path):
    bash, results, final, init, cost = [], [], "", {}, None
    for line in open(path):
        try:
            e = json.loads(line)
        except ValueError:
            continue
        if e.get("type") == "system" and e.get("subtype") == "init":
            init = {k: e.get(k) for k in ("tools", "mcp_servers", "skills", "slash_commands", "model")}
        if e.get("type") == "assistant":
            for c in e["message"].get("content", []):
                if c.get("type") == "tool_use" and c.get("name") == "Bash":
                    bash.append(c["input"].get("command", ""))
        if e.get("type") == "user":
            for c in (e.get("message", {}).get("content") or []):
                if isinstance(c, dict) and c.get("type") == "tool_result":
                    t = c.get("content")
                    results.append(t if isinstance(t, str) else json.dumps(t))
        if e.get("type") == "result":
            final = e.get("result", "")
            cost = e
    return bash, results, final, init, cost


def check(task, home, bash, results, final):
    if task == "T1":
        return "bench-db" in final and any("credential dbms list" in c for c in bash)
    if task == "T2":
        r = cli(home, "config", "get", "format")
        try:
            return json.loads(r.stdout).get("format") == "json"
        except ValueError:
            return r.stdout.strip().strip('"') == "json"
    if task == "T3":
        ok = [c for c in bash if "config list" in c and re.search(r"--format[ =](json|table|toon|default)", c)]
        return bool(ok) and len(bash) <= 4
    if task == "T4":
        d = os.path.join(home, ".claude", "skills")
        return os.path.isdir(d) and any(f == "SKILL.md" for _, _, fs in os.walk(d) for f in fs)
    if task == "T5":
        need = {"create", "delete", "deploy", "load", "overwrite", "pause", "resume", "update"}
        words = set(re.findall(r"[a-z]+", final.lower()))
        return need <= words and not ({"get", "list"} & words)
    if task == "T6":
        m = re.search(r"\[.*\]", final, re.S)
        try:
            arr = json.loads(m.group(0)) if m else None
        except ValueError:
            return False
        return isinstance(arr, list) and len(arr) == 5 and all(isinstance(x, str) and x.startswith("neo4j-cli") for x in arr)
    if task == "T7":
        ran = lambda w: any(w in c for c in bash)
        return "42" in final and ran("docker create") and ran(" query") and not managed_containers()
    return False


def main():
    task, n = sys.argv[1], sys.argv[2]
    sha = hashlib.sha256(open(BIN, "rb").read()).hexdigest()
    pinned = P["pins"]["neo4j_cli"]["sha256"]
    out = os.path.join(ROOT, "results", f"{task}-{n}")
    shutil.rmtree(out, ignore_errors=True)
    os.makedirs(out)
    run = tempfile.mkdtemp(prefix=f"swf-{task}-")
    home, work, bindir = (os.path.join(run, d) for d in ("home", "work", "bin"))
    os.makedirs(work); os.makedirs(bindir)
    wrapper = os.path.join(bindir, "neo4j-cli")
    docker_host = f'DOCKER_HOST="unix://{os.path.expanduser("~")}/.docker/run/docker.sock" ' if task == "T7" else ""
    open(wrapper, "w").write(f'#!/bin/sh\n{docker_host}HOME="{home}" exec "{BIN}" "$@"\n')
    os.chmod(wrapper, 0o755)
    setup(task, home)
    env = dict(os.environ, PATH=bindir + ":" + os.environ["PATH"])
    env.pop("CLAUDE_CODE_SIMPLE", None)
    cmd = ["claude", "-p", PROMPTS[task], "--model", MODEL, "--output-format", "stream-json", "--verbose",
           "--tools", "Bash", "--allowedTools", "Bash(neo4j-cli:*)", "Bash(head:*)", "Bash(tail:*)", "Bash(jq:*)", "Bash(wc:*)", "--setting-sources", "project",
           "--disable-slash-commands", "--strict-mcp-config", "--no-session-persistence",
           "--permission-mode", "dontAsk", "--max-budget-usd", "2",
           "--append-system-prompt", "The shell only allows the neo4j-cli command and the head, tail, jq and wc commands. Any other command is denied. Use neo4j-cli itself to find what you need."]
    t0 = time.time()
    status = "ok"
    with open(os.path.join(out, "stream.jsonl"), "w") as f:
        try:
            subprocess.run(cmd, cwd=work, env=env, stdout=f, stderr=subprocess.PIPE, timeout=H["timeout_seconds"])
        except subprocess.TimeoutExpired:
            status = "timeout"
    secs = round(time.time() - t0, 1)
    bash, results, final, init, res = parse_stream(os.path.join(out, "stream.jsonl"))
    success = status == "ok" and check(task, home, bash, results, final)
    if task == "T7" and managed_containers():
        subprocess.run(["docker", "rm", "-f", *managed_containers()], capture_output=True)
    usage = (res or {}).get("usage", {})
    rec = {
        "task": task, "run": int(n), "status": status, "success": bool(success),
        "binary_sha_matches_pin": sha == pinned,
        "seconds": secs, "turns": (res or {}).get("num_turns"), "bash_calls": len(bash),
        "cost_usd": (res or {}).get("total_cost_usd"),
        "tokens": {k: usage.get(k) for k in ("input_tokens", "output_tokens", "cache_read_input_tokens", "cache_creation_input_tokens")},
        "session_init": init, "commands": bash, "final": final[:2000],
    }
    json.dump(rec, open(os.path.join(out, "result.json"), "w"), indent=1)
    print(json.dumps({k: rec[k] for k in ("task", "run", "status", "success", "seconds", "turns", "bash_calls", "cost_usd")}))
    shutil.rmtree(run, ignore_errors=True)


main()
