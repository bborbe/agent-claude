#!/usr/bin/env python3
"""Hermetic unit tests for the GitHub App git credential helper.

No network and no ambient `GITHUB_APP_*` dependency: the API host and the HTTP
seam are monkeypatched, and every invocation gets an explicit environment. The
only external process is `openssl`, which is required -- the signing round-trip
is the point of the suite, so a missing `openssl` is a failure, not a skip.

Run: python3 -m unittest discover -s scripts -t scripts -p '*_test.py' -v
"""

import base64
import contextlib
import io
import json
import os
import re
import subprocess
import sys
import tempfile
import textwrap
import unittest
import urllib.error
from unittest import mock

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import git_credential_github_app as helper  # noqa: E402

FAKE_TOKEN = "ghs_fakeinstallationtoken"
FAKE_JWT = "JWT-SENTINEL-DO-NOT-LEAK"
BASE64URL = re.compile(r"^[A-Za-z0-9_-]+$")
CREDENTIAL_INPUT = "protocol=https\nhost=github.com\n\n"


def generate_rsa_key():
    """Return a throwaway 2048-bit RSA private key in PEM form."""
    result = subprocess.run(["openssl", "genrsa", "2048"], capture_output=True)
    if result.returncode != 0:
        raise AssertionError(f"openssl genrsa failed: {result.stderr!r}")
    return result.stdout


def b64url_decode(segment):
    """Decode a padding-stripped base64url segment."""
    return base64.urlsafe_b64decode(segment + "=" * (-len(segment) % 4))


def verify_signature(jwt_token, key_pem):
    """Return True when the JWT signature verifies against the key's public half."""
    header_segment, payload_segment, signature_segment = jwt_token.split(".")
    signing_input = f"{header_segment}.{payload_segment}".encode("ascii")
    public = subprocess.run(["openssl", "rsa", "-pubout"], input=key_pem, capture_output=True)
    if public.returncode != 0:
        raise AssertionError(f"openssl rsa -pubout failed: {public.stderr!r}")
    with tempfile.TemporaryDirectory() as directory:
        public_path = os.path.join(directory, "public.pem")
        signature_path = os.path.join(directory, "signature.bin")
        with open(public_path, "wb") as handle:
            handle.write(public.stdout)
        with open(signature_path, "wb") as handle:
            handle.write(b64url_decode(signature_segment))
        result = subprocess.run(
            ["openssl", "dgst", "-sha256", "-verify", public_path, "-signature", signature_path],
            input=signing_input,
            capture_output=True,
        )
    return result.returncode == 0


@contextlib.contextmanager
def explicit_environment(values):
    """Apply exactly the given GITHUB_APP_* values, hiding any ambient ones."""
    saved = {name: os.environ.get(name) for name in helper.APP_ENV_NAMES}
    for name in helper.APP_ENV_NAMES:
        os.environ.pop(name, None)
    for name, value in values.items():
        os.environ[name] = value
    try:
        yield
    finally:
        for name in helper.APP_ENV_NAMES:
            os.environ.pop(name, None)
        for name, value in saved.items():
            if value is not None:
                os.environ[name] = value


def run_helper(argv, values, stdin_text=""):
    """Run helper.main in-process and return (exit_code, stdout, stderr)."""
    stdout, stderr = io.StringIO(), io.StringIO()
    with explicit_environment(values), mock.patch("sys.stdin", io.StringIO(stdin_text)):
        with contextlib.redirect_stdout(stdout), contextlib.redirect_stderr(stderr):
            code = helper.main(argv)
    return code, stdout.getvalue(), stderr.getvalue()


class GitCredentialGithubAppTest(unittest.TestCase):
    """Exercise the credential-helper protocol, the mint and every failure path."""

    @classmethod
    def setUpClass(cls):
        cls.key_pem = generate_rsa_key()
        cls.key_b64 = base64.b64encode(cls.key_pem).decode("ascii")

    def valid_env(self):
        """Return a complete, valid environment mapping."""
        return {
            "GITHUB_APP_ID": "1",
            "GITHUB_APP_INSTALLATION_ID": "2",
            "GITHUB_APP_PEM": self.key_b64,
        }

    def test_get_writes_username_and_password(self):
        response = helper.ApiResponse(200, {}, {"token": FAKE_TOKEN})
        with mock.patch.object(helper, "post_json", return_value=response):
            code, out, err = run_helper(["get"], self.valid_env(), CREDENTIAL_INPUT)
        self.assertEqual(code, 0)
        self.assertEqual(out, f"username=x-access-token\npassword={FAKE_TOKEN}\n")
        self.assertEqual(err, "")

    def test_token_writes_bare_token(self):
        response = helper.ApiResponse(200, {}, {"token": FAKE_TOKEN})
        with mock.patch.object(helper, "post_json", return_value=response):
            code, out, err = run_helper(["token"], self.valid_env())
        self.assertEqual(code, 0)
        self.assertEqual(out, f"{FAKE_TOKEN}\n")
        self.assertEqual(err, "")

    def test_store_and_erase_are_silent_noops(self):
        for command in ("store", "erase"):
            with self.subTest(command=command):
                code, out, err = run_helper([command], {}, CREDENTIAL_INPUT)
                self.assertEqual(code, 0)
                self.assertEqual(out, "")
                self.assertEqual(err, "")

    def test_bad_subcommand_is_rejected(self):
        for argv in (["bogus"], []):
            with self.subTest(argv=argv):
                code, out, err = run_helper(argv, self.valid_env(), CREDENTIAL_INPUT)
                self.assertNotEqual(code, 0)
                self.assertEqual(out, "")
                self.assertNotEqual(err, "")

    def test_missing_or_empty_variables_fail_loudly(self):
        for name in helper.APP_ENV_NAMES:
            for label, value in (("unset", None), ("empty", "")):
                with self.subTest(name=name, label=label):
                    values = self.valid_env()
                    if value is None:
                        del values[name]
                    else:
                        values[name] = value
                    code, out, err = run_helper(["get"], values, CREDENTIAL_INPUT)
                    self.assertEqual(code, 1)
                    self.assertIn(name, err)
                    self.assertNotIn("password=", out)

    def test_undecodable_pem_fails_loudly(self):
        values = self.valid_env()
        values["GITHUB_APP_PEM"] = "!!!not-base64!!!"
        code, out, err = run_helper(["token"], values)
        self.assertEqual(code, 1)
        self.assertIn("GITHUB_APP_PEM", err)
        self.assertEqual(out, "")

    def test_build_jwt_has_expected_shape_and_signature(self):
        now = 1700000000
        token = helper.build_jwt("123", self.key_pem, now)
        segments = token.split(".")
        self.assertEqual(len(segments), 3)
        for segment in segments:
            self.assertRegex(segment, BASE64URL)
            self.assertNotIn("=", segment)
        self.assertEqual(
            json.loads(b64url_decode(segments[0])), {"alg": "RS256", "typ": "JWT"}
        )
        payload = json.loads(b64url_decode(segments[1]))
        self.assertEqual(payload["iss"], "123")
        self.assertEqual(payload["iat"], now - 60)
        self.assertEqual(payload["exp"], now + 540)
        self.assertLess(payload["exp"] - now, 600)
        self.assertTrue(verify_signature(token, self.key_pem))

    def test_wrapped_base64_pem_mints(self):
        values = self.valid_env()
        values["GITHUB_APP_PEM"] = "\n".join(textwrap.wrap(self.key_b64, 76))
        response = helper.ApiResponse(200, {}, {"token": FAKE_TOKEN})
        with mock.patch.object(helper, "post_json", return_value=response):
            code, out, err = run_helper(["token"], values)
        self.assertEqual(code, 0)
        self.assertEqual(out, f"{FAKE_TOKEN}\n")
        self.assertEqual(err, "")

    def test_raw_pem_mints(self):
        """The pod delivers the PEM itself, not base64.

        The Secret template renders `teamvaultFile | base64` into the Secret's
        `.data`, and Kubernetes DECODES `.data` when injecting an environment
        variable -- so the pod's GITHUB_APP_PEM is the PEM. Requiring base64
        here left the helper unable to mint in the pod at all, while every
        mocked test passed. Measured 2026-10-09 against the rendered Secret.
        """
        values = self.valid_env()
        values["GITHUB_APP_PEM"] = self.key_pem.decode("ascii")
        response = helper.ApiResponse(200, {}, {"token": FAKE_TOKEN})
        with mock.patch.object(helper, "post_json", return_value=response):
            code, out, err = run_helper(["token"], values)
        self.assertEqual(code, 0)
        self.assertEqual(out, f"{FAKE_TOKEN}\n")
        self.assertEqual(err, "")

    def test_base64_that_is_not_a_pem_fails_loudly(self):
        values = self.valid_env()
        values["GITHUB_APP_PEM"] = base64.b64encode(b"not a pem at all").decode("ascii")
        code, out, err = run_helper(["token"], values)
        self.assertEqual(code, 1)
        self.assertIn("GITHUB_APP_PEM", err)
        self.assertEqual(out, "")

    def test_api_failure_classes_map_to_exit_codes(self):
        cases = (
            ("rejected-jwt", helper.ApiResponse(401, {}, {"message": "Bad credentials"}), 2),
            (
                "throttled",
                helper.ApiResponse(403, {"X-RateLimit-Remaining": "0"}, {"message": "rate limited"}),
                3,
            ),
            ("server-error", helper.ApiResponse(500, {}, {"message": "boom"}), 5),
            ("no-token-field", helper.ApiResponse(201, {}, {"expires_at": "later"}), 5),
        )
        for label, response, expected in cases:
            with self.subTest(label=label):
                with mock.patch.object(helper, "post_json", return_value=response):
                    code, out, err = run_helper(["token"], self.valid_env())
                self.assertEqual(code, expected)
                self.assertEqual(out, "")
                self.assertEqual(err.strip(), err.strip().splitlines()[0])
                self.assertNotEqual(err.strip(), "")

    def test_transport_failure_exits_four(self):
        failure = urllib.error.URLError("no route to host")
        with mock.patch.object(helper, "post_json", side_effect=failure):
            code, out, err = run_helper(["token"], self.valid_env())
        self.assertEqual(code, 4)
        self.assertEqual(out, "")
        self.assertNotEqual(err.strip(), "")

    def test_failure_paths_never_leak_credentials(self):
        captured = {}

        def recording_post(url, payload, headers):
            captured["authorization"] = headers["Authorization"]
            return helper.ApiResponse(500, {}, {"message": "boom"})

        with mock.patch.object(helper, "post_json", side_effect=recording_post):
            code, out, err = run_helper(["token"], self.valid_env())
        self.assertEqual(code, 5)
        attempted_jwt = captured["authorization"].removeprefix("Bearer ")
        combined = out + err
        self.assertNotIn(attempted_jwt, combined)
        self.assertNotIn(self.key_b64, combined)
        self.assertNotIn("PRIVATE KEY", combined)

        with mock.patch.object(helper, "post_json", return_value=helper.ApiResponse(401, {}, {})):
            with mock.patch.object(helper, "build_jwt", return_value=FAKE_JWT):
                code, out, err = run_helper(["get"], self.valid_env(), CREDENTIAL_INPUT)
        self.assertEqual(code, 2)
        self.assertNotIn(FAKE_JWT, out + err)
        self.assertNotIn("password=", out)

    def test_api_base_is_a_module_constant(self):
        self.assertEqual(helper.API_BASE, "https://api.github.com")
        values = self.valid_env()
        values["GITHUB_API_URL"] = "https://evil.example.com"
        response = helper.ApiResponse(200, {}, {"token": FAKE_TOKEN})
        urls = []

        def recording_post(url, payload, headers):
            urls.append(url)
            return response

        with mock.patch.object(helper, "post_json", side_effect=recording_post):
            code, _, _ = run_helper(["token"], values)
        self.assertEqual(code, 0)
        self.assertEqual(urls, ["https://api.github.com/app/installations/2/access_tokens"])


if __name__ == "__main__":
    unittest.main()
