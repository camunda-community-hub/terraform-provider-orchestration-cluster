#!/usr/bin/env python3

"""Post-generation fixes for oapi-codegen output.

Fixes known issues in the generated client code:
1. String literal assignments to *string pointer fields (discriminator defaults)
2. Duplicate type declarations (model vs response wrapper name collision)
"""

import re
import sys

FILE = sys.argv[1] if len(sys.argv) > 1 else "pkg/camunda/client.gen.go"

def main():
    with open(FILE, "r") as f:
        code = f.read()

    # 1. Add ptrStr helper if not already present
    if "func ptrStr(" not in code:
        code += "\n// ptrStr returns a pointer to the given string value.\nfunc ptrStr(s string) *string { return &s }\n"

    # 2. Fix string-to-*string discriminator assignments
    # Pattern: v.Type = "someValue" where Type is *string
    code = re.sub(r'\bv\.Type = ("[A-Za-z_]+")$', r'v.Type = ptrStr(\1)', code, flags=re.MULTILINE)

    # 3. Fix model types that collide with a response wrapper of the same name
    # (an operation <Op> whose response schema is called <Op>Response). The model is declared
    # first and the wrapper (which has Body and HTTPResponse fields) second; rename the wrapper
    # to <Op>Resp and keep the model, which the wrapper's JSON<status> fields refer to.
    names = re.findall(r'^type (\w+) struct \{$', code, re.MULTILINE)
    for name in sorted({n for n in names if names.count(n) > 1}):
        resp = name[: -len("Response")] + "Resp" if name.endswith("Response") else name + "Resp"
        code = code.replace("type %s struct {\n\tBody " % name, "type %s struct {\n\tBody " % resp)
        code = code.replace("func (r %s) " % name, "func (r %s) " % resp)
        code = code.replace("(*%s, error)" % name, "(*%s, error)" % resp)
        code = code.replace("&%s{" % name, "&%s{" % resp)
        code = code.replace("func Parse%s(" % name, "func Parse%s(" % resp)
        code = code.replace("Parse%s(rsp)" % name, "Parse%s(rsp)" % resp)
        code = code.replace("Parse%s(" % name, "Parse%s(" % resp)

    with open(FILE, "w") as f:
        f.write(code)

    print(f"Post-generation fixes applied to {FILE}")

if __name__ == "__main__":
    main()
