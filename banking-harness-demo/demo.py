"""Narrated walkthrough: see what the model proposed and what the harness decided.

    python demo.py                         # the four headline scenarios
    python demo.py injection_known_payee   # any scenario by name
    python demo.py --list
"""

import json
import sys
import textwrap

from evals.run import run_scenario
from evals.scenarios import SCENARIOS

HEADLINE = ["small_transfer", "approval_declined", "injection_known_payee", "runaway_loop"]

ICON = {"user_message": "CUSTOMER ", "tool_executed": "EXECUTED ", "policy_denied": "DENIED   ",
        "approval": "APPROVAL ", "tool_rejected": "REJECTED ", "tool_failed": "FAILED   ",
        "injection_suspected": "ALERT    ", "loop_detected": "STOPPED  ", "assistant_reply": "ASSISTANT",
        "budget_exhausted": "STOPPED  ", "model_stopped": "STOPPED  "}


def describe(e):
    k = e["kind"]
    if k == "user_message":
        return e["text"]
    if k == "assistant_reply":
        return e["text"]
    args = json.dumps(e.get("args", {}))
    if k == "tool_executed":
        res = e["result"] if e["result"] == "<read>" else json.dumps(e["result"])
        return f"{e['tool']}({args}) [{e['risk']}] -> {res}"
    if k == "policy_denied":
        return f"{e['tool']}({args}) because: {'; '.join(e['reasons'])}"
    if k == "approval":
        verdict = "APPROVED" if e["approved"] else "DECLINED"
        return f"human reviewer {verdict} {e['tool']}({args}); asked because: {'; '.join(e['reasons'])}"
    if k == "tool_rejected":
        return f"{e['tool']}: {e['why']} {e.get('errors', '')}"
    if k == "injection_suspected":
        return f"prompt-injection pattern in {e['source']}; session tainted (money movement now needs review)"
    if k == "loop_detected":
        return f"{e['tool']} called {e['repeats']}x with identical arguments; run halted and escalated"
    return json.dumps(e)


def show(sc):
    print("=" * 100)
    print(f"{sc.name}: {sc.what_it_shows}")
    print("=" * 100)
    ctx, failed = run_scenario(sc)
    for e in ctx.audit.events:
        label = ICON.get(e["kind"], e["kind"][:9].upper())
        body = textwrap.fill(describe(e), 88, subsequent_indent=" " * 12)
        print(f"  {label}  {body}")
    status = "PASS" if not failed else "FAIL: " + "; ".join(failed)
    print(f"\n  checks: {status}\n")


def main(args):
    if "--list" in args:
        for s in SCENARIOS:
            print(f"{s.name:<26}{s.what_it_shows}")
        return
    names = args or HEADLINE
    by_name = {s.name: s for s in SCENARIOS}
    for n in names:
        show(by_name[n])


if __name__ == "__main__":
    main(sys.argv[1:])
