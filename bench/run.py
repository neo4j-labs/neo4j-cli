#!/usr/bin/env python3
"""neo4j-cli agent benchmark runner: python3 -I bench/run.py <task> <run-number> [--bin PATH --arm LABEL --model NAME].
Writes bench/results/<task>-<n>/ for the pinned build and model, bench/results/<arm>/<task>-<n>/ for a
non-pinned binary, and bench/results/<model-slug>/<task>-<n>/ when only the model differs from the pin."""
import argparse, hashlib, json, os, re, shutil, subprocess, sys, time, tempfile

ROOT = os.path.dirname(os.path.abspath(__file__))
P = json.load(open(os.path.join(ROOT, "protocol.json")))
ap = argparse.ArgumentParser()
ap.add_argument("task"); ap.add_argument("run")
ap.add_argument("--bin", help="neo4j-cli binary to test instead of the pinned release (needs --arm)")
ap.add_argument("--arm", help="label for the binary, e.g. parent or changed")
ap.add_argument("--model", help="model to run instead of the pinned one; results go to results/<model-slug>/ unless --arm is given")
ARGS = ap.parse_args()
if bool(ARGS.bin) != bool(ARGS.arm):
    ap.error("--bin and --arm go together")
PINNED = P["pins"]["neo4j_cli"]
BIN = os.path.abspath(ARGS.bin) if ARGS.bin else PINNED["path"]
MODEL = ARGS.model or P["pins"]["model"]
SLUG = re.sub(r"[^A-Za-z0-9._-]+", "-", MODEL)
SUBDIR = ARGS.arm or (SLUG if (ARGS.model and MODEL != P["pins"]["model"]) else None)
H = P["harness"]

TASKS = {t["id"]: t for t in json.load(open(os.path.join(ROOT, "tasks.json")))}


AURA = H["aura"]
T10 = {}  # id of the instance the T10 setup creates
AURA_VARS = ("AURA_CLIENT_ID", "AURA_CLIENT_SECRET")


def aura_credentials():
    """Client id and secret from the environment, else from ~/.config/aura/env (KEY=VALUE lines)."""
    found = {k: os.environ.get(k, "") for k in AURA_VARS}
    path = os.path.expanduser("~/.config/aura/env")
    if not all(found.values()) and os.path.exists(path):
        for line in open(path):
            k, _, v = line.strip().removeprefix("export ").partition("=")
            if k in found and not found[k]:
                found[k] = v.strip().strip("\"'")
    if not all(found.values()):
        sys.exit(f"{' and '.join(AURA_VARS)} must be set in the environment or in ~/.config/aura/env")
    return found


def cli(home, *args):
    env = dict(os.environ, HOME=home)
    return subprocess.run([BIN, *args], env=env, capture_output=True, text=True)


def setup_aura(home):
    creds = aura_credentials()
    file = os.path.join(os.path.dirname(home), "aura-client.env")
    fd = os.open(file, os.O_WRONLY | os.O_CREAT, 0o600)
    with os.fdopen(fd, "w") as f:
        f.write(f"CLIENT_ID={creds['AURA_CLIENT_ID']}\nCLIENT_SECRET={creds['AURA_CLIENT_SECRET']}\nCLIENT_NAME={AURA['credential_name']}\n")
    r = cli(home, "credential", "aura-client", "add", "--name", AURA["credential_name"], "--env", file, "--rw")
    os.remove(file)
    if r.returncode:
        sys.exit(f"could not add the aura-client credential: {r.stderr.strip()[:300]}")
    for key, value in (("aura.base-url", AURA["base_url"]), ("aura.auth-url", AURA["auth_url"]), ("aura.default-workspace", AURA["workspace"])):
        cli(home, "config", "set", key, value, "--rw")
    remove_bench_instances(home)


def instances(home):
    r = cli(home, "aura", "instance", "list", "--format", "json")
    try:
        return json.loads(r.stdout)["data"]
    except (ValueError, KeyError):
        return None


def remove_bench_instances(home):
    """Delete every bench-* instance and wait until the workspace is empty (a free workspace holds one instance)."""
    for i in instances(home) or []:
        if i["name"].startswith("bench-"):
            cli(home, "aura", "instance", "delete", i["id"], "--rw", "--yes", "--force")
    for _ in range(60):
        if not instances(home):
            return
        time.sleep(5)
    sys.exit("the dev workspace still holds instances after cleanup")


def setup(task, home):
    os.makedirs(home, exist_ok=True)
    cli(home, "config", "set", "credential-storage", "insecure", "--rw")
    if TASKS[task].get("aura"):
        setup_aura(home)
    if task == "T1":
        cli(home, "credential", "dbms", "add", "--name", "bench-db", "--uri", "neo4j://localhost:7687",
            "--username", "neo4j", "--password", "bench-pass", "--rw")
    if task == "T10":
        r = cli(home, "aura", "instance", "create", "--type", "free", "--name", f"bench-t10-{ARGS.run}", "--wait",
                "--no-credential-print", "--no-credential-storage", "--rw")
        if r.returncode:
            sys.exit(f"could not create the T10 instance: {r.stderr.strip()[:300]}")
        T10["id"] = named_instance(home, f"bench-t10-{ARGS.run}")["id"]
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


def has_word(text, word):
    return re.search(r"(?<![\w-])" + re.escape(word) + r"(?![\w-])", text) is not None


def check_t2(home, bash, final):
    r = cli(home, "config", "get", "format")
    try:
        return json.loads(r.stdout).get("format") == "json"
    except ValueError:
        return r.stdout.strip().strip('"') == "json"


def check_t3(home, bash, final):
    ok = [c for c in bash if "config list" in c and re.search(r"--format[ =](json|table|toon|default)", c)]
    return bool(ok) and len(bash) <= 4


def check_t4(home, bash, final):
    d = os.path.join(home, ".claude", "skills")
    return os.path.isdir(d) and any(f == "SKILL.md" for _, _, fs in os.walk(d) for f in fs)


def check_t5(home, bash, final):
    need = {"create", "delete", "deploy", "load", "overwrite", "pause", "resume", "update"}
    words = set(re.findall(r"[a-z]+", final.lower()))
    return need <= words and not ({"get", "list"} & words)


def check_t6(home, bash, final):
    m = re.search(r"\[.*\]", final, re.S)
    try:
        arr = json.loads(m.group(0)) if m else None
    except ValueError:
        return False
    return isinstance(arr, list) and len(arr) == 5 and all(isinstance(x, str) and x.startswith("neo4j-cli") for x in arr)


def check_t7(home, bash, final):
    return not managed_containers()


def named_instance(home, name):
    return next((i for i in instances(home) or [] if i["name"] == name), None)


def instance_status(home, id):
    """The list endpoint has no status; get does. None when the instance no longer exists."""
    r = cli(home, "aura", "instance", "get", id, "--format", "json")
    try:
        return json.loads(r.stdout)["data"]["status"]
    except (ValueError, KeyError):
        return None


def check_t9(home, bash, final):
    i = named_instance(home, f"bench-t9-{ARGS.run}")
    return bool(i) and instance_status(home, i["id"]) == "running" and has_word(final, i["id"])


def check_t10(home, bash, final):
    i = named_instance(home, f"bench-t10-{ARGS.run}")
    gone = i is None or instance_status(home, i["id"]) in (None, "destroying")
    return gone and has_word(final, T10["id"])


STATE_CHECKS = {"T2": check_t2, "T3": check_t3, "T4": check_t4, "T5": check_t5, "T6": check_t6, "T7": check_t7,
                "T9": check_t9, "T10": check_t10}


def check(task, home, bash, results, final):
    t = TASKS[task]
    if "expectedAnswer" in t and not has_word(final, t["expectedAnswer"]):
        return False
    if not all(any(m in c for c in bash) for m in t.get("mustRun", [])):
        return False
    return STATE_CHECKS.get(task, lambda *a: True)(home, bash, final)


def main():
    task, n = ARGS.task, ARGS.run
    sha = hashlib.sha256(open(BIN, "rb").read()).hexdigest()
    out = os.path.join(ROOT, "results", *([SUBDIR] if SUBDIR else []), f"{task}-{n}")
    shutil.rmtree(out, ignore_errors=True)
    os.makedirs(out)
    version = subprocess.run([BIN, "--version"], capture_output=True, text=True).stdout.strip()
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
    for k in AURA_VARS:
        env.pop(k, None)
    cmd = ["claude", "-p", TASKS[task]["prompt"].replace("{run}", n), "--model", MODEL, "--output-format", "stream-json", "--verbose",
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
    if TASKS[task].get("aura"):
        remove_bench_instances(home)
    if task == "T7" and managed_containers():
        subprocess.run(["docker", "rm", "-f", *managed_containers()], capture_output=True)
    usage = (res or {}).get("usage", {})
    rec = {
        "task": task, "run": int(n), "status": status, "success": bool(success),
        "arm": ARGS.arm, "model": MODEL,
        "binary": BIN, "binary_version": version, "binary_sha256": sha,
        "binary_sha_matches_pin": sha == PINNED["sha256"] if not ARGS.bin else None,
        "tool_output_bytes": sum(len(r.encode()) for r in results),
        "seconds": secs, "turns": (res or {}).get("num_turns"), "bash_calls": len(bash),
        "cost_usd": (res or {}).get("total_cost_usd"),
        "tokens": {k: usage.get(k) for k in ("input_tokens", "output_tokens", "cache_read_input_tokens", "cache_creation_input_tokens")},
        "session_init": init, "commands": bash, "final": final[:2000],
    }
    json.dump(rec, open(os.path.join(out, "result.json"), "w"), indent=1)
    print(json.dumps({k: rec[k] for k in ("task", "run", "status", "success", "seconds", "turns", "bash_calls", "tool_output_bytes", "cost_usd")}))
    shutil.rmtree(run, ignore_errors=True)


main()
