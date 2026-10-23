#!/bin/sh
# Container entrypoint: mint GH_TOKEN from the GitHub App credential when one is
# present, then hand over to /main unchanged.
#
# WHY THIS EXISTS
# The image ships `gh` so an agent can drive GitHub without a bespoke client.
# `gh` authenticates from GH_TOKEN (or a hosts file), and neither existed: the
# App PEM was wired only into `git` through the credential helper, so `gh` would
# have run unauthenticated and every call would have failed 401. The token is
# minted here, once per container, from the SAME credential and the SAME code
# path `git` uses.
#
# WHY IT DEGRADES INSTEAD OF FAILING
# This is a shared image: most agents carry no GitHub App at all, and their
# `GITHUB_APP_*` vars are simply unset. A hard failure here would take down
# every agent that has no App credential. So the mint is attempted only when all
# three vars are present, and any failure leaves GH_TOKEN unset rather than
# aborting the container — the agent still starts, and `gh` still reports its own
# auth error if it is actually invoked.
#
# ⚠️ The token expires after one hour. That is longer than any single agent
# phase, and a job that outlives it fails on `gh` with a 401 rather than silently
# acting as a stale identity. Re-minting per invocation is deliberately not done
# here: `gh` reads the variable once at process start, so a wrapper would have to
# intercept every call, and the expiry would still be bounded by the phase.
set -eu

if [ -n "${GITHUB_APP_ID:-}" ] && [ -n "${GITHUB_APP_INSTALLATION_ID:-}" ] && [ -n "${GITHUB_APP_PEM:-}" ]; then
	# The helper's stderr is kept and printed on failure rather than discarded:
	# "GITHUB_APP_ID is unset" and "GitHub rejected the JWT (HTTP 401)" are the
	# difference between a misconfigured pod and a transient API failure, and the
	# mint runs once per container, so a silent failure costs the pod its gh
	# credential for its whole life. The helper never writes the token to stderr,
	# so forwarding it cannot leak the credential.
	if token=$(/usr/local/bin/gh-token 2>/tmp/gh-token.err) && [ -n "$token" ]; then
		GH_TOKEN="$token"
		export GH_TOKEN
	else
		echo "entrypoint: gh-token produced no token — continuing without GH_TOKEN" >&2
		cat /tmp/gh-token.err >&2 2>/dev/null || true
	fi
	rm -f /tmp/gh-token.err
fi

exec /main "$@"
