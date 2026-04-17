#!/usr/bin/env python3
"""启动性能分析脚本。

跑 release 二进制，解析 tracing 的 INFO 输出时间戳，
把相邻两条 info 之间的耗时归到对应阶段，多次采样取中位数。
标准库依赖，Python 3.8+。
"""
from __future__ import annotations

import argparse
import json
import os
import re
import signal
import statistics
import subprocess
import sys
import time
from datetime import datetime
from pathlib import Path
from typing import Optional

ANSI_RE = re.compile(r"\x1b\[[0-9;]*m")
TS_PATTERN = re.compile(
    r"^(?P<ts>\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z)\s+INFO\s+\S+:\s+(?P<msg>.*)$"
)

PHASE_RULES: list[tuple[re.Pattern, str]] = [
    (re.compile(r"^database:\s"), "config_load + tracing_init"),
    (re.compile(r"^DATABASE_DSN not set, starting postgres"), "docker_compose_up"),
    (re.compile(r"^DATABASE_DSN not set, using compose postgres"), "docker_inside_wait"),
    (re.compile(r"^postgres is ready at"), "postgres_connect"),
    (re.compile(r"^postgres database .* already exists"), "db_exists_check"),
    (re.compile(r"^created postgres database"), "db_create"),
    (re.compile(r"^using redis cache"), "db_migrate + pool_init + cache"),
    (re.compile(r"^redis unavailable"), "db_migrate + pool_init + cache"),
    (re.compile(r"^no redis configured"), "db_migrate + pool_init + cache"),
    (re.compile(r"^cleared stale rate-limit fields"), "account_store_cleanup"),
    (re.compile(r"^claude-code-gateway listening on"), "services_init + router_build + bind"),
]

LISTENING_RE = re.compile(r"^claude-code-gateway listening on")


def parse_ts(s: str) -> float:
    m = re.match(r"^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(?:\.(\d+))?Z$", s)
    if not m:
        raise ValueError(f"bad timestamp: {s!r}")
    base, frac = m.group(1), m.group(2) or ""
    frac = (frac + "000000")[:6]
    return datetime.fromisoformat(f"{base}.{frac}+00:00").timestamp()


def classify(msg: str) -> Optional[str]:
    for pat, name in PHASE_RULES:
        if pat.match(msg):
            return name
    return None


def profile_once(bin_path: Path, cwd: Path, timeout: float) -> dict:
    t0 = time.time()
    proc = subprocess.Popen(
        [str(bin_path)],
        cwd=str(cwd),
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        env=os.environ.copy(),
        text=True,
        bufsize=1,
        start_new_session=True,
    )

    events: list[tuple[float, str, str]] = []
    raw_tail: list[str] = []
    deadline = t0 + timeout
    listening_seen = False
    try:
        assert proc.stdout is not None
        while True:
            if time.time() > deadline:
                break
            line = proc.stdout.readline()
            if not line:
                if proc.poll() is not None:
                    break
                continue
            line = line.rstrip("\n")
            raw_tail.append(line)
            if len(raw_tail) > 40:
                raw_tail.pop(0)
            clean = ANSI_RE.sub("", line)
            m = TS_PATTERN.match(clean)
            if not m:
                continue
            try:
                ts = parse_ts(m.group("ts"))
            except ValueError:
                continue
            msg = m.group("msg")
            phase = classify(msg)
            if phase is None:
                continue
            events.append((ts, msg, phase))
            if LISTENING_RE.match(msg):
                listening_seen = True
                break
    finally:
        if proc.poll() is None:
            try:
                os.killpg(proc.pid, signal.SIGTERM)
                proc.wait(timeout=5)
            except Exception:
                try:
                    os.killpg(proc.pid, signal.SIGKILL)
                except Exception:
                    pass

    if not listening_seen or not events:
        return {
            "ok": False,
            "error": "listening log not observed within timeout",
            "raw_tail": raw_tail[-20:],
        }

    phases: list[tuple[str, float]] = []
    first_ts, first_msg, first_phase = events[0]
    phases.append((first_phase, first_ts - t0))
    for i in range(1, len(events)):
        prev_ts = events[i - 1][0]
        cur_ts, _, cur_phase = events[i]
        phases.append((cur_phase, cur_ts - prev_ts))

    total = events[-1][0] - t0
    return {"ok": True, "phases": phases, "total": total}


def aggregate(samples: list[dict]) -> dict:
    good = [s for s in samples if s["ok"]]
    if not good:
        return {"ok": False, "runs": len(samples)}
    per_phase: dict[str, list[float]] = {}
    totals: list[float] = []
    for s in good:
        for name, dt in s["phases"]:
            per_phase.setdefault(name, []).append(dt)
        totals.append(s["total"])
    agg = []
    for name, vs in per_phase.items():
        agg.append(
            {
                "phase": name,
                "median": statistics.median(vs),
                "min": min(vs),
                "max": max(vs),
                "count": len(vs),
            }
        )
    agg.sort(key=lambda x: x["median"], reverse=True)
    return {
        "ok": True,
        "runs": len(samples),
        "runs_ok": len(good),
        "phases": agg,
        "total_median": statistics.median(totals),
        "total_min": min(totals),
        "total_max": max(totals),
    }


def render_table(agg: dict) -> str:
    if not agg.get("ok"):
        return f"[no successful samples among {agg.get('runs', 0)} runs]"
    total_median = agg["total_median"]
    phases = agg["phases"]
    width = max(len(p["phase"]) for p in phases)
    width = max(width, len("phase"))

    def fmt_ms(x: float) -> str:
        return f"{x * 1000:8.1f} ms"

    lines = []
    header = f"    {'phase':<{width}}  {'median':>11}  {'min':>11}  {'max':>11}  {'share':>6}"
    lines.append(header)
    lines.append("    " + "-" * (len(header) - 4))
    for i, p in enumerate(phases):
        marker = ">>> " if i == 0 else "    "
        share = (p["median"] / total_median * 100) if total_median > 0 else 0
        lines.append(
            f"{marker}{p['phase']:<{width}}  {fmt_ms(p['median'])}  {fmt_ms(p['min'])}  {fmt_ms(p['max'])}  {share:5.1f}%"
        )
    lines.append(
        f"    {'total':<{width}}  {fmt_ms(total_median)}  {fmt_ms(agg['total_min'])}  {fmt_ms(agg['total_max'])}"
    )
    lines.append(f"    (runs: {agg['runs_ok']}/{agg['runs']} ok)")
    return "\n".join(lines)


def main() -> int:
    ap = argparse.ArgumentParser(description="cc-bridge startup profiler")
    ap.add_argument("--runs", type=int, default=3)
    ap.add_argument("--rebuild", action="store_true", help="cargo build --release before profiling")
    ap.add_argument("--json", dest="json_out", type=str, default=None)
    ap.add_argument("--bin", dest="bin_path", type=str, default=None)
    ap.add_argument("--timeout", type=float, default=60.0)
    args = ap.parse_args()

    repo_root = Path(__file__).resolve().parent.parent
    default_bin = repo_root / "target" / "release" / "claude-code-gateway"
    bin_path = Path(args.bin_path).resolve() if args.bin_path else default_bin

    if args.rebuild or not bin_path.exists():
        print(f"[build] cargo build --release (cwd={repo_root})", flush=True)
        r = subprocess.run(["cargo", "build", "--release"], cwd=str(repo_root))
        if r.returncode != 0:
            print("[build] failed", file=sys.stderr)
            return r.returncode
    if not bin_path.exists():
        print(f"[error] binary not found: {bin_path}", file=sys.stderr)
        return 2

    samples: list[dict] = []
    for i in range(args.runs):
        print(f"[run {i + 1}/{args.runs}] {bin_path}", flush=True)
        s = profile_once(bin_path, repo_root, args.timeout)
        if s["ok"]:
            print(f"         total {s['total'] * 1000:.1f} ms, {len(s['phases'])} phases", flush=True)
        else:
            print(f"         FAILED: {s.get('error')}", flush=True)
            for line in s.get("raw_tail", []):
                print(f"         | {line}", flush=True)
        samples.append(s)

    agg = aggregate(samples)
    print()
    print(render_table(agg))

    if args.json_out:
        out = {
            "samples": samples,
            "aggregate": agg,
            "bin": str(bin_path),
            "repo_root": str(repo_root),
        }
        Path(args.json_out).write_text(json.dumps(out, indent=2, ensure_ascii=False))
        print(f"\n[json] wrote {args.json_out}", flush=True)

    return 0 if agg.get("ok") else 1


if __name__ == "__main__":
    sys.exit(main())
