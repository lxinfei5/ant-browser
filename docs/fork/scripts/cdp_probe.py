#!/usr/bin/env python3
"""Minimal Playwright CDP probe for Ant-Browser LaunchServer.

Auth note (post-hardening):
  The LaunchServer binds 127.0.0.1 only. Since the security-hardening fork,
  `launch_server.auth.enabled` is ON BY DEFAULT and a key is auto-generated on
  first run and written to the app's config.yaml at the macOS stateRoot
  (~/Library/Application Support/ant-browser/config.yaml). The state-changing
  /api/* routes (start/stop) now require the `X-Ant-Api-Key` header, so this
  probe auto-reads that key from config unless you pass --api-key / ANT_API_KEY.
  To restore the old no-key local flow, set `launch_server.auth.enabled: false`
  in the stateRoot config (NOT the repo config.yaml — wails dev runs in
  production mode and reads the stateRoot config). Local CDP transport itself
  (connect_over_cdp) still works without a key.

Usage:
  python cdp_probe.py <instance_code> [--base http://127.0.0.1:19876] [--api-key KEY]
  # instance_code = the Profile's launch code (resolved by LaunchCodeService)
"""
import argparse
import os
import re
import sys
import requests
from playwright.sync_api import sync_playwright

DEFAULT_CONFIG = os.path.expanduser("~/Library/Application Support/ant-browser/config.yaml")


def load_api_key_from_config(path):
    """Best-effort parse of launch_server.auth.api_key from the app config.yaml
    (the stateRoot copy, not the repo copy). Avoids a PyYAML dependency."""
    try:
        text = open(path, "r", encoding="utf-8").read()
    except OSError:
        return None
    # find the launch_server: block, then auth:, then api_key:
    m = re.search(r"^launch_server:\s*$([\s\S]*?)(?=^\S|\Z)", text, re.M)
    if not m:
        return None
    block = m.group(1)
    ma = re.search(r"^\s*api_key:\s*['\"]?([^'\"\n#]+?)['\"]?\s*(?:#.*)?$", block, re.M)
    return ma.group(1).strip() if ma else None


def main():
    ap = argparse.ArgumentParser(description="Ant-Browser CDP probe via LaunchServer")
    ap.add_argument("code", help="instance launch code (Profile launchCode)")
    ap.add_argument("--base", default="http://127.0.0.1:19876",
                    help="LaunchServer base URL (config.yaml launch_server.port, default 19876)")
    ap.add_argument("--api-key", default=os.environ.get("ANT_API_KEY"),
                    help="X-Ant-Api-Key; if unset, auto-read from the app config (default-on auth)")
    ap.add_argument("--config", default=DEFAULT_CONFIG,
                    help=f"app config.yaml to read the key from (default {DEFAULT_CONFIG})")
    ap.add_argument("--url", default="https://browserleaks.com/javascript",
                    help="page to open (default browserleaks javascript)")
    ap.add_argument("--timeout-ms", type=int, default=30000)
    args = ap.parse_args()

    api_key = args.api_key or load_api_key_from_config(args.config)
    headers = {"Content-Type": "application/json"}
    if api_key:
        headers["X-Ant-Api-Key"] = api_key
    else:
        print("[warn] no API key found (not in --api-key/ANT_API_KEY, not in config). "
              "Start/stop will 403 unless launch_server.auth.enabled is false.", file=sys.stderr)

    # 1) Start the instance (by launch code) and wait for debug-ready.
    r = requests.post(f"{args.base}/api/runtime/session",
                      headers=headers,
                      json={"code": args.code, "timeoutMs": args.timeout_ms})
    r.raise_for_status()
    body = r.json()
    if not body.get("ready"):
        print(f"[warn] instance not ready: {body}", file=sys.stderr)
    # The REAL field name carrying the CDP URL is `cdpUrl` (lowercase, http://127.0.0.1:{port}).
    cdp_url = body.get("cdpUrl")
    if not cdp_url:
        sys.exit(f"no cdpUrl in response: {body}")
    print(f"[ok] cdpUrl = {cdp_url}")

    # 2) Connect over CDP and open a page.
    with sync_playwright() as p:
        browser = p.chromium.connect_over_cdp(cdp_url)  # http base; Playwright resolves /json
        ctx = browser.contexts[0] if browser.contexts else browser.new_context()
        page = ctx.new_page()
        page.goto(args.url)
        title = page.title()
        ua = page.evaluate("navigator.userAgent")
        lang = page.evaluate("navigator.language")
        print(f"[page] title={title!r}")
        print(f"[hint] ua={ua!r} lang={lang!r}")  # fingerprint hint

    # 3) Stop the instance.
    s = requests.post(f"{args.base}/api/runtime/stop",
                      headers=headers, json={"code": args.code})
    print(f"[stop] status={s.status_code} body={s.text}")


if __name__ == "__main__":
    main()