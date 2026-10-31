"""Minimal JSON Schema validation for tool arguments (stdlib only).

Supports the subset the tools use: type, required, additionalProperties,
enum, pattern, minimum/exclusiveMinimum/maximum, maxLength. Model output is
untrusted input: validate before anything executes.
"""

import re

_TYPES = {"object": dict, "string": str, "integer": int, "number": (int, float), "boolean": bool}


def validate(schema: dict, value, path: str = "$") -> list[str]:
    errs = []
    t = schema.get("type")
    if t:
        ok = isinstance(value, _TYPES[t]) and not (t in ("integer", "number") and isinstance(value, bool))
        if not ok:
            return [f"{path}: expected {t}, got {type(value).__name__} ({value!r})"]
    if t == "object":
        props = schema.get("properties", {})
        for req in schema.get("required", []):
            if req not in value:
                errs.append(f"{path}.{req}: required")
        if schema.get("additionalProperties") is False:
            errs += [f"{path}.{k}: unexpected property" for k in value if k not in props]
        for k, sub in props.items():
            if k in value:
                errs += validate(sub, value[k], f"{path}.{k}")
    if "enum" in schema and value not in schema["enum"]:
        errs.append(f"{path}: must be one of {schema['enum']}")
    if isinstance(value, str):
        if "pattern" in schema and not re.fullmatch(schema["pattern"], value):
            errs.append(f"{path}: {value!r} does not match {schema['pattern']}")
        if "maxLength" in schema and len(value) > schema["maxLength"]:
            errs.append(f"{path}: longer than {schema['maxLength']} characters")
    if isinstance(value, (int, float)) and not isinstance(value, bool):
        if "minimum" in schema and value < schema["minimum"]:
            errs.append(f"{path}: must be >= {schema['minimum']}")
        if "exclusiveMinimum" in schema and value <= schema["exclusiveMinimum"]:
            errs.append(f"{path}: must be > {schema['exclusiveMinimum']}")
        if "maximum" in schema and value > schema["maximum"]:
            errs.append(f"{path}: must be <= {schema['maximum']}")
    return errs
