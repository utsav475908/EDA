"""Run the eval suite.

    python -m evals.run                      # scripted model, all scenarios (offline, deterministic)
    python -m evals.run --model claude       # live Claude on scenarios marked live=True
    python -m evals.run -k injection         # filter by name
    python -m evals.run --disable policy     # ablation: switch a harness layer off
    python -m evals.run --ablate             # ablate every layer, one at a time
"""

import argparse
import sys
from pathlib import Path
from types import SimpleNamespace

from harness.bank import Bank
from harness.loop import LAYERS, AuditLog, Harness, Session
from harness.models import ScriptedModel

from .scenarios import SCENARIOS

RUNS = Path(__file__).resolve().parent.parent / "runs"


def run_scenario(sc, model_name="scripted", audit_dir=RUNS, disabled=()):
    bank = Bank()
    approvals = []

    def approver(req):
        approvals.append(req)
        return sc.approver == "approve"

    model = ScriptedModel(sc.script) if model_name == "scripted" else _claude()
    audit_dir.mkdir(exist_ok=True)
    path = audit_dir / f"{sc.name}.{model_name}.jsonl"
    path.unlink(missing_ok=True)
    audit = AuditLog(path)
    session = Session(customer_id=sc.customer)
    start = {a: v["balance"] for a, v in bank.accounts.items()}
    result = Harness(bank, model, approver, audit, disabled=disabled).run(session, sc.user)
    ctx = SimpleNamespace(bank=bank, start=start, audit=audit, result=result, session=session, approvals=approvals)
    failed = [label for label, check in sc.checks if not check(ctx)]
    return ctx, failed


def _claude():
    from harness.models import ClaudeModel
    return ClaudeModel()


def main(argv=None):
    p = argparse.ArgumentParser()
    p.add_argument("--model", choices=["scripted", "claude"], default="scripted")
    p.add_argument("-k", default="", help="only scenarios whose name contains this")
    p.add_argument("--disable", default="", help=f"comma-separated layers to switch off: {','.join(LAYERS)}")
    p.add_argument("--ablate", action="store_true", help="disable each layer in turn and report what breaks")
    args = p.parse_args(argv)

    chosen = [s for s in SCENARIOS if args.k in s.name and (args.model == "scripted" or s.live)]
    if args.ablate:
        return ablate(chosen, args.model)
    disabled = [x for x in args.disable.split(",") if x]
    if disabled:
        print(f"!! harness layers disabled: {', '.join(disabled)}\n")
    print(f"{'scenario':<26}{'result':<8}{'steps':>5}{'tools':>6}{'tokens':>8}  notes")
    print("-" * 90)
    passed = 0
    for sc in chosen:
        ctx, failed = run_scenario(sc, args.model, disabled=disabled)
        ok = not failed
        passed += ok
        u = ctx.result.usage
        note = "" if ok else "FAILED: " + "; ".join(failed)
        print(f"{sc.name:<26}{'PASS' if ok else 'FAIL':<8}{ctx.result.steps:>5}{len(ctx.result.tool_calls):>6}"
              f"{u['input_tokens'] + u['output_tokens']:>8}  {note}")
    print("-" * 90)
    print(f"{passed}/{len(chosen)} passed   (audit logs: {RUNS}/)")
    return 0 if passed == len(chosen) else 1


def ablate(chosen, model):
    """What does each layer buy us? Scenarios that fail when it's switched off."""
    print(f"{'layer disabled':<16}{'scenarios that break':<5}")
    print("-" * 90)
    for layer in LAYERS:
        broke = [sc.name for sc in chosen if run_scenario(sc, model, disabled=[layer])[1]]
        print(f"{layer:<16}{', '.join(broke) or '(none)'}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
