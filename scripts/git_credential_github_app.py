#!/usr/bin/env python3
"""Git credential helper that mints a GitHub App installation token on demand.

`git` invokes a credential helper itself, so the minted token is handed to `git`
over the helper protocol on stdin/stdout and never lands in a remote URL,
`.git/config`, `~/.git-credentials`, `~/.netrc` or an argv. The identity comes
from exactly three environment variables -- `GITHUB_APP_ID`,
`GITHUB_APP_INSTALLATION_ID` and `GITHUB_APP_PEM` (the PEM key itself; base64
is also accepted). The API host is the module constant below, so nothing in the
environment can steer the helper at a different App, installation or host.

  get     git credential-helper protocol; prints username and password
  store   no-op; nothing is ever persisted
  erase   no-op; there is no cache to erase
  token   prints the bare installation token

Nothing is cached: every invocation that needs a token mints a fresh one, so two
concurrent invocations simply each mint their own and cannot interfere. Every
failure exits non-zero with a single-line diagnostic on stderr naming the
offending input, and writes no `password=` line -- a helper that returned an
empty credential would let `git` fall back to an anonymous request, which
succeeds against a public repo and fails against a private one.

Run: python3 git_credential_github_app.py get
     python3 git_credential_github_app.py token
"""

import base64
import binascii
import collections
import json
import os
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request

# The API host is a module constant and is never read from the environment: a
# worker must not be able to steer the minted token at an arbitrary endpoint.
API_BASE = "https://api.github.com"

ACCESS_TOKENS_PATH = "/app/installations/{installation_id}/access_tokens"

REQUEST_TIMEOUT_SECONDS = 30

# GitHub rejects a JWT whose `exp` is more than 10 minutes ahead; 9 minutes
# leaves a margin so clock skew cannot invalidate the token.
JWT_BACKDATE_SECONDS = 60
JWT_LIFETIME_SECONDS = 540

CREDENTIAL_USERNAME = "x-access-token"

EXIT_LOCAL_INPUT = 1
EXIT_JWT_REJECTED = 2
EXIT_THROTTLED = 3
EXIT_TRANSPORT = 4
EXIT_API_ERROR = 5

APP_ENV_NAMES = ("GITHUB_APP_ID", "GITHUB_APP_INSTALLATION_ID", "GITHUB_APP_PEM")

# A base64-encoded GITHUB_APP_PEM may wrap at 76 columns, so embedded newlines
# are normal input rather than corruption when the value arrives in that shape.
ASCII_WHITESPACE = " \t\n\r\v\f"

ApiResponse = collections.namedtuple("ApiResponse", ["status", "headers", "body"])


class HelperError(Exception):
    """A failure that carries the process exit code for its class."""

    def __init__(self, message, exit_code):
        super().__init__(message)
        self.exit_code = exit_code


def read_env(environ):
    """Return (app_id, installation_id, pem_value), or raise HelperError naming the gap."""
    values = {}
    for name in APP_ENV_NAMES:
        value = environ.get(name)
        if value is None or value == "":
            raise HelperError(f"{name} is unset or empty", EXIT_LOCAL_INPUT)
        values[name] = value
    return (
        values["GITHUB_APP_ID"],
        values["GITHUB_APP_INSTALLATION_ID"],
        values["GITHUB_APP_PEM"],
    )


def decode_pem(value):
    """Return the PEM bytes from GITHUB_APP_PEM, accepting either shape.

    The deployment delivers the key as RAW PEM, not base64: the Secret template
    renders `teamvaultFile | base64` into the Secret's `.data`, and Kubernetes
    DECODES `.data` when it injects the value as an environment variable -- so
    the pod's `GITHUB_APP_PEM` is the PEM itself. Measured 2026-10-09 against
    the rendered `claude-agent` Secret: `.data.GITHUB_APP_PEM` base64-decodes to
    a 1675-byte `-----BEGIN RSA PRIVATE KEY-----`, which is what the pod sees.

    Base64 is still accepted, because that is what a Secret looks like before
    injection and what an operator pasting a value by hand would supply. Both
    shapes are handled deliberately: assuming either one alone leaves the pod
    holding a credential it cannot use, which is the inert-credential failure
    this helper exists to remove.
    """
    if "-----BEGIN" in value:
        return value.encode("utf-8")
    stripped = "".join(char for char in value if char not in ASCII_WHITESPACE)
    try:
        decoded = base64.b64decode(stripped, validate=True)
    except (binascii.Error, ValueError) as err:
        raise HelperError(
            f"GITHUB_APP_PEM is neither a PEM nor decodable base64: {err}",
            EXIT_LOCAL_INPUT,
        )
    if b"-----BEGIN" not in decoded:
        raise HelperError(
            "GITHUB_APP_PEM base64-decoded but is not a PEM private key",
            EXIT_LOCAL_INPUT,
        )
    return decoded


def build_jwt(app_id, pem_bytes, now):
    """Return a signed RS256 JWT for the App, valid from now-60s to now+540s."""
    header = {"alg": "RS256", "typ": "JWT"}
    payload = {
        "iat": now - JWT_BACKDATE_SECONDS,
        "exp": now + JWT_LIFETIME_SECONDS,
        "iss": app_id,
    }
    signing_input = b".".join([_b64url(_json_bytes(header)), _b64url(_json_bytes(payload))])
    signature = _sign(signing_input, pem_bytes)
    return b".".join([signing_input, _b64url(signature)]).decode("ascii")


def post_json(url, payload, headers):
    """POST JSON and return an ApiResponse; HTTP errors are returned, not raised."""
    body = json.dumps(payload).encode("utf-8")
    request = urllib.request.Request(url, data=body, headers=headers, method="POST")
    try:
        with urllib.request.urlopen(request, timeout=REQUEST_TIMEOUT_SECONDS) as response:
            return ApiResponse(
                response.status,
                _normalise_headers(response.headers),
                _parse_body(response.read()),
            )
    except urllib.error.HTTPError as err:
        return ApiResponse(err.code, _normalise_headers(err.headers), _parse_body(err.read()))


def mint_installation_token(app_id, installation_id, pem_bytes):
    """Exchange a freshly signed JWT for an installation token, or raise HelperError."""
    now = int(time.time())
    jwt_token = build_jwt(app_id, pem_bytes, now)
    url = API_BASE + ACCESS_TOKENS_PATH.format(installation_id=installation_id)
    headers = {
        "Authorization": f"Bearer {jwt_token}",
        "Accept": "application/vnd.github+json",
        "X-GitHub-Api-Version": "2022-11-28",
        "Content-Type": "application/json",
    }
    try:
        response = post_json(url, {}, headers)
    except (urllib.error.URLError, OSError) as err:
        raise HelperError(f"failed to reach {API_BASE}: {err}", EXIT_TRANSPORT)

    status = response.status
    if 200 <= status < 300:
        token = response.body.get("token")
        if not token:
            raise HelperError(
                f"GitHub returned HTTP {status} without an installation token",
                EXIT_API_ERROR,
            )
        return token
    if status == 401:
        raise HelperError(
            "GitHub rejected the JWT (HTTP 401); check that GITHUB_APP_ID matches GITHUB_APP_PEM",
            EXIT_JWT_REJECTED,
        )
    if status == 403 and _is_throttled(response):
        raise HelperError("GitHub throttled the installation-token request (HTTP 403)", EXIT_THROTTLED)
    raise HelperError(
        f"GitHub returned HTTP {status} for the installation-token request",
        EXIT_API_ERROR,
    )


def main(argv):
    """Dispatch a credential-helper invocation and return the process exit code."""
    if not argv:
        return _fail("usage: git_credential_github_app.py {get|store|erase|token}")
    command = argv[0]
    if command in ("store", "erase"):
        return 0
    if command not in ("get", "token"):
        return _fail(f"unknown subcommand: {command}")

    try:
        app_id, installation_id, pem_value = read_env(os.environ)
        pem_bytes = decode_pem(pem_value)
        if command == "get":
            _read_credential_input()
        token = mint_installation_token(app_id, installation_id, pem_bytes)
    except HelperError as err:
        return _fail(str(err), err.exit_code)

    if command == "get":
        sys.stdout.write(f"username={CREDENTIAL_USERNAME}\n")
        sys.stdout.write(f"password={token}\n")
    else:
        sys.stdout.write(f"{token}\n")
    return 0


def _b64url(data):
    """Base64url-encode bytes with the padding stripped."""
    return base64.urlsafe_b64encode(data).rstrip(b"=")


def _json_bytes(value):
    """Serialise a JSON object to compact ASCII bytes."""
    return json.dumps(value, separators=(",", ":")).encode("ascii")


def _sign(signing_input, pem_bytes):
    """Sign the signing input with openssl, holding the key in a 0600 temp file."""
    handle, key_path = tempfile.mkstemp()
    try:
        with os.fdopen(handle, "wb") as key_file:
            key_file.write(pem_bytes)
        os.chmod(key_path, 0o600)
        try:
            result = subprocess.run(
                ["openssl", "dgst", "-sha256", "-sign", key_path],
                input=signing_input,
                capture_output=True,
            )
        except OSError as err:
            raise HelperError(f"openssl is unavailable to sign with GITHUB_APP_PEM: {err}", EXIT_LOCAL_INPUT)
        if result.returncode != 0:
            raise HelperError("openssl failed to sign the JWT with GITHUB_APP_PEM", EXIT_LOCAL_INPUT)
        return result.stdout
    finally:
        try:
            os.remove(key_path)
        except OSError:
            pass


def _normalise_headers(headers):
    """Return the response headers as a dict with lowercased keys."""
    return {key.lower(): value for key, value in headers.items()}


def _parse_body(raw):
    """Parse a response body into a dict, returning {} when it is not a JSON object."""
    try:
        parsed = json.loads(raw.decode("utf-8"))
    except (UnicodeDecodeError, ValueError):
        return {}
    return parsed if isinstance(parsed, dict) else {}


def _is_throttled(response):
    """Report whether a 403 is a rate-limit response rather than an auth failure."""
    for key, value in response.headers.items():
        if key.lower() == "x-ratelimit-remaining" and str(value) == "0":
            return True
    message = response.body.get("message")
    return isinstance(message, str) and "rate limit" in message.lower()


def _read_credential_input():
    """Drain the credential-helper request git writes to stdin."""
    try:
        sys.stdin.read()
    except (OSError, ValueError):
        pass


def _fail(message, exit_code=EXIT_LOCAL_INPUT):
    """Write a single diagnostic line to stderr and return the exit code."""
    sys.stderr.write(f"{message}\n")
    return exit_code


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
