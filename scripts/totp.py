#!/usr/bin/env python3
"""TOTP codes for the development analysts (RFC 6238, like an authenticator app).

    scripts/totp.py                 # current code of each analyst
    scripts/totp.py --consume EMAIL # a fresh code for EMAIL, for scripts

A code is accepted once and refused codes count towards the lockout, so
--consume keeps the last step used per analyst in a temp file, the same one
the Go e2e and Playwright tests use, and never hands out a used step (it waits
for the next one when needed).
"""
import base64, hashlib, hmac, os, struct, sys, tempfile, time

ANALYSTS = {
    "analista@digitalbank.dev": "JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP",
    "revisora@digitalbank.dev": "KRSXG5CTMVRXEZLUKRSXG5CTMVRXEZLU",
}


def code(secret: str, step: int) -> str:
    mac = hmac.new(base64.b32decode(secret), struct.pack(">Q", step), hashlib.sha1).digest()
    o = mac[-1] & 0x0F
    return f"{(struct.unpack('>I', mac[o:o + 4])[0] & 0x7FFFFFFF) % 1_000_000:06d}"


def consume(email: str) -> str:
    path = os.path.join(tempfile.gettempdir(), f"digital-bank-totp-{email}")
    try:
        last = int(open(path).read().strip() or 0)
    except (OSError, ValueError):
        last = 0
    now = int(time.time()) // 30
    step = max(now, last + 1)
    if step > now + 1:  # both usable steps are taken: wait for the next one
        time.sleep((step - 1) * 30 - time.time() + 0.5)
    with open(path, "w") as f:
        f.write(str(step))
    return code(ANALYSTS[email], step)


if __name__ == "__main__":
    if len(sys.argv) == 3 and sys.argv[1] == "--consume":
        print(consume(sys.argv[2]))
    else:
        left = 30 - int(time.time()) % 30
        for email, secret in ANALYSTS.items():
            print(f"{email}  {code(secret, int(time.time()) // 30)}  (vale por {left}s)")
