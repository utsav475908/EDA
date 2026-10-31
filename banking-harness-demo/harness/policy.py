"""Policy engine: the rules that decide whether a tool call may run.

Rules live in code, not in the prompt. The model can be confused, jailbroken
or simply wrong; the policy engine cannot be talked out of a limit.
Identity always comes from the authenticated session, never from model args.
"""

from dataclasses import dataclass, field
from datetime import timedelta

ALLOW, APPROVE, DENY = "allow", "require_approval", "deny"

AUTO_APPROVE_LIMIT = 1_000.00      # above this, a human must approve
SINGLE_TRANSFER_LIMIT = 5_000.00   # above this, refuse (branch / relationship manager)
DAILY_TRANSFER_LIMIT = 10_000.00
NEW_PAYEE_COOLING = timedelta(hours=24)


@dataclass
class Decision:
    verdict: str
    reasons: list[str] = field(default_factory=list)


def _owns_account(bank, session, account_id) -> bool:
    acct = bank.accounts.get(account_id)
    return bool(acct) and acct["customer_id"] == session.customer_id


def check_account_read(bank, session, args) -> Decision:
    # "Not found" rather than "forbidden": don't confirm other customers' accounts exist.
    if not _owns_account(bank, session, args["account_id"]):
        return Decision(DENY, [f"account {args['account_id']} not found for this customer"])
    return Decision(ALLOW)


def check_transfer(bank, session, args) -> Decision:
    acct_id, ben_id, amount = args["from_account_id"], args["beneficiary_id"], args["amount"]
    customer = bank.customers[session.customer_id]

    deny = []
    if not _owns_account(bank, session, acct_id):
        deny.append(f"account {acct_id} not found for this customer")
    if customer["kyc_status"] != "verified":
        deny.append("customer KYC is not verified; transfers are disabled")
    ben = bank.beneficiaries.get(ben_id)
    if not ben or ben["customer_id"] != session.customer_id:
        deny.append(f"beneficiary {ben_id} is not registered for this customer; "
                    "new payees must be added in the banking app with 2FA")
    if amount > SINGLE_TRANSFER_LIMIT:
        deny.append(f"amount exceeds the {SINGLE_TRANSFER_LIMIT:,.0f} single-transfer limit")
    if not deny:
        acct = bank.accounts[acct_id]
        if acct["status"] != "active":
            deny.append(f"account {acct_id} is {acct['status']}")
        elif amount > acct["balance"]:
            deny.append("insufficient funds")
        if bank.transferred_today(session.customer_id) + amount > DAILY_TRANSFER_LIMIT:
            deny.append(f"daily transfer limit of {DAILY_TRANSFER_LIMIT:,.0f} would be exceeded")
    if deny:
        return Decision(DENY, deny)

    step_up = []
    if amount > AUTO_APPROVE_LIMIT:
        step_up.append(f"amount above {AUTO_APPROVE_LIMIT:,.0f} auto-approval limit")
    if bank.beneficiary_age(ben_id) < NEW_PAYEE_COOLING:
        step_up.append("beneficiary added less than 24 hours ago")
    if session.tainted:
        step_up.append("session has read suspected prompt-injection content")
    return Decision(APPROVE, step_up) if step_up else Decision(ALLOW)


def check_card(bank, session, args) -> Decision:
    if bank.card_owner(args["card_id"]) != session.customer_id:
        return Decision(DENY, [f"card {args['card_id']} not found for this customer"])
    return Decision(ALLOW)
