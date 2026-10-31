"""Model adapters. The harness only depends on `respond(system, tools, messages)`.

ScriptedModel  - deterministic stand-in for an LLM. Each scenario scripts what a
                 (good, careless or compromised) model would emit, so harness
                 behaviour can be demonstrated and regression-tested offline.
ClaudeModel    - the real thing, via the Anthropic Python SDK (pip install anthropic).
"""

import itertools
import json

_ids = itertools.count(1)


def tool(name: str, **inp) -> dict:
    return {"type": "tool_use", "id": f"toolu_{next(_ids):04d}", "name": name, "input": inp}


def say(text: str) -> dict:
    return {"type": "text", "text": text}


def last_results(messages) -> list[dict]:
    """Parsed tool results from the most recent harness message."""
    for m in reversed(messages):
        if m["role"] == "user" and isinstance(m["content"], list):
            return [{"is_error": r.get("is_error", False), **json.loads(r["content"])} for r in m["content"]]
    return []


class ScriptedModel:
    """Plays back a script. A step is a list of blocks, or a function of the
    conversation returning blocks (to react to tool results like a model would)."""

    def __init__(self, steps):
        self.steps = list(steps)

    def respond(self, system, tools, messages):
        step = self.steps.pop(0) if self.steps else [say("(script ended)")]
        blocks = step(messages) if callable(step) else step
        stop = "tool_use" if any(b["type"] == "tool_use" for b in blocks) else "end_turn"
        tokens_in = sum(len(json.dumps(m["content"], default=str)) for m in messages) // 4
        return {"stop_reason": stop, "content": blocks,
                "usage": {"input_tokens": tokens_in, "output_tokens": len(json.dumps(blocks)) // 4}}


class ClaudeModel:
    """Claude Opus 5.5 with server-side refusal fallback enabled.

    The harness owns the loop (manual agentic loop), which is the point of the
    demo: every tool call passes through validation, policy, approval and audit.
    """

    def __init__(self, model: str = "claude-opus-5-5", effort: str = "medium"):
        import anthropic  # optional dependency

        self.client = anthropic.Anthropic()
        self.model, self.effort = model, effort

    def respond(self, system, tools, messages):
        resp = self.client.beta.messages.create(
            model=self.model,
            max_tokens=16000,
            system=system,
            tools=tools,
            messages=messages,
            output_config={"effort": self.effort},
            betas=["server-side-fallback-2026-07-01"],
            extra_body={"fallbacks": "default"},
        )
        return {
            "stop_reason": resp.stop_reason,
            # Keep every block (incl. thinking) so it can be passed back unchanged.
            "content": [b.model_dump(exclude_none=True) for b in resp.content],
            "usage": {"input_tokens": resp.usage.input_tokens, "output_tokens": resp.usage.output_tokens},
        }
