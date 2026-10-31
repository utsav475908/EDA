"""In-memory core banking system (the system of record the agent acts on).

This layer knows nothing about LLMs. It enforces only what a real core
system would: balances, account state, idempotent posting.
"""

import copy
import json
from datetime import datetime, timedelta
from pathlib import Path

DATA = Path(__file__).resolve().parent.parent / "data" / "bank.json"


class BankError(Exception):
    pass


class Bank:
    def __init__(self, path: Path = DATA):
        self._seed = json.loads(Path(path).read_text())
        self.reset()

    def reset(self):
        d = copy.deepcopy(self._seed)
        self.now = datetime.fromisoformat(d["as_of"])
        self.currency = d["currency"]
        self.customers = {c["customer_id"]: c for c in d["customers"]}
        self.accounts = {a["account_id"]: a for a in d["accounts"]}
        self.cards = {c["card_id"]: c for c in d["cards"]}
        self.beneficiaries = {b["beneficiary_id"]: b for b in d["beneficiaries"]}
        self.transactions = d["transactions"]
        self.posted_keys: dict[str, dict] = {}  # idempotency key -> posted transfer

    # ---- reads ----
    def accounts_of(self, customer_id: str) -> list[dict]:
        return [a for a in self.accounts.values() if a["customer_id"] == customer_id]

    def transactions_of(self, account_id: str, limit: int) -> list[dict]:
        rows = [t for t in self.transactions if t["account_id"] == account_id]
        return rows[-limit:][::-1]

    def beneficiaries_of(self, customer_id: str) -> list[dict]:
        return [b for b in self.beneficiaries.values() if b["customer_id"] == customer_id]

    def transferred_today(self, customer_id: str) -> float:
        start = self.now.replace(hour=0, minute=0, second=0, microsecond=0)
        own = {a["account_id"] for a in self.accounts_of(customer_id)}
        return sum(-t["amount"] for t in self.transactions
                   if t["account_id"] in own and t["category"] == "transfer_out"
                   and datetime.fromisoformat(t["posted_at"]) >= start)

    def beneficiary_age(self, beneficiary_id: str) -> timedelta:
        return self.now - datetime.fromisoformat(self.beneficiaries[beneficiary_id]["added_at"])

    # ---- writes ----
    def transfer(self, from_account_id: str, beneficiary_id: str, amount: float,
                 memo: str, idempotency_key: str) -> dict:
        if idempotency_key in self.posted_keys:
            return {**self.posted_keys[idempotency_key], "duplicate_request": True}
        acct = self.accounts[from_account_id]
        if acct["status"] != "active":
            raise BankError(f"account {from_account_id} is {acct['status']}")
        if amount > acct["balance"]:
            raise BankError("insufficient funds")
        acct["balance"] = round(acct["balance"] - amount, 2)
        txn = {
            "txn_id": f"TXN-{len(self.transactions) + 1:05d}", "account_id": from_account_id,
            "posted_at": self.now.isoformat(), "amount": -amount, "direction": "debit",
            "counterparty": self.beneficiaries[beneficiary_id]["name"], "category": "transfer_out",
            "memo": memo,
        }
        self.transactions.append(txn)
        receipt = {"status": "posted", "txn_id": txn["txn_id"], "amount": amount,
                   "new_balance": acct["balance"], "duplicate_request": False}
        self.posted_keys[idempotency_key] = receipt
        return receipt

    def freeze_card(self, card_id: str) -> dict:
        self.cards[card_id]["status"] = "frozen"
        return {"status": "frozen", "card_id": card_id}

    def card_owner(self, card_id: str) -> str | None:
        card = self.cards.get(card_id)
        return self.accounts[card["account_id"]]["customer_id"] if card else None
