#!/usr/bin/env python3
"""
Generate Microsoft Graph delegated tokens for a personal Outlook account.

Prerequisites:
  pip install selenium requests

Edit the constants below, then run:
  python outlook_tokens.py
"""

from __future__ import annotations

import base64
import hashlib
import json
import secrets
import sys
import time
from pathlib import Path
from urllib.parse import parse_qs, urlencode, urlparse

import requests
from selenium import webdriver
from selenium.common.exceptions import NoSuchElementException, TimeoutException
from selenium.webdriver.common.by import By
from selenium.webdriver.common.keys import Keys
from selenium.webdriver.support import expected_conditions as EC
from selenium.webdriver.support.ui import WebDriverWait


CLIENT_ID = "3"
CLIENT_SECRET = ""
USERNAME = ""
PASSWORD = ""

TENANT = "common"
REDIRECT_URI = "https://login.microsoftonline.com/common/oauth2/nativeclient"
SCOPE = "offline_access Mail.Send User.Read"
OUTPUT_FILE = "tokens.json"
LOGIN_TIMEOUT_SECONDS = 180


def pkce_pair() -> tuple[str, str]:
    verifier = base64.urlsafe_b64encode(secrets.token_bytes(32)).rstrip(b"=").decode("ascii")
    digest = hashlib.sha256(verifier.encode("ascii")).digest()
    challenge = base64.urlsafe_b64encode(digest).rstrip(b"=").decode("ascii")
    return verifier, challenge


def build_auth_url(state: str, code_challenge: str) -> str:
    query = urlencode(
        {
            "client_id": CLIENT_ID,
            "response_type": "code",
            "redirect_uri": REDIRECT_URI,
            "response_mode": "query",
            "scope": SCOPE,
            "state": state,
            "code_challenge": code_challenge,
            "code_challenge_method": "S256",
        }
    )
    return f"https://login.microsoftonline.com/{TENANT}/oauth2/v2.0/authorize?{query}"


def maybe_type_credentials(driver: webdriver.Chrome) -> None:
    if not USERNAME or not PASSWORD:
        print("USERNAME/PASSWORD are empty. Complete Microsoft sign-in manually in the browser.")
        return

    wait = WebDriverWait(driver, 30)
    username_input = wait.until(EC.presence_of_element_located((By.NAME, "loginfmt")))
    username_input.clear()
    username_input.send_keys(USERNAME)
    username_input.send_keys(Keys.RETURN)

    password_input = wait.until(EC.presence_of_element_located((By.NAME, "passwd")))
    password_input.clear()
    password_input.send_keys(PASSWORD)
    password_input.send_keys(Keys.RETURN)

    try:
        stay_signed_in = WebDriverWait(driver, 10).until(
            EC.element_to_be_clickable((By.ID, "idBtn_Back"))
        )
        stay_signed_in.click()
    except TimeoutException:
        pass

    try:
        accept = WebDriverWait(driver, 10).until(
            EC.element_to_be_clickable((By.ID, "idBtn_Accept"))
        )
        accept.click()
    except TimeoutException:
        pass


def parse_auth_code(callback_url: str, expected_state: str) -> str | None:
    parsed = urlparse(callback_url)
    values = parse_qs(parsed.query)
    if "error" in values:
        description = values.get("error_description", values["error"])[0]
        raise RuntimeError(f"Microsoft authorization failed: {description}")
    if "code" not in values:
        return None
    state = values.get("state", [""])[0]
    if state != expected_state:
        raise RuntimeError("OAuth state mismatch")
    return values["code"][0]


def authenticate() -> tuple[str, str]:
    state = secrets.token_urlsafe(24)
    code_verifier, code_challenge = pkce_pair()
    auth_url = build_auth_url(state, code_challenge)

    try:
        driver = webdriver.Chrome()
    except Exception as exc:
        raise RuntimeError(
            "Chrome WebDriver failed to start. On Windows, run: "
            "python -m pip install -U selenium urllib3 requests"
        ) from exc
    try:
        driver.get(auth_url)
        maybe_type_credentials(driver)
        deadline = time.time() + LOGIN_TIMEOUT_SECONDS
        while time.time() < deadline:
            code = parse_auth_code(driver.current_url, state)
            if code:
                return code, code_verifier
            time.sleep(1)
    finally:
        driver.quit()

    raise TimeoutError("Timed out waiting for Microsoft authorization code")


def exchange_code(code: str, code_verifier: str) -> dict:
    token_data = {
        "grant_type": "authorization_code",
        "code": code,
        "redirect_uri": REDIRECT_URI,
        "client_id": CLIENT_ID,
        "scope": SCOPE,
        "code_verifier": code_verifier,
    }
    if CLIENT_SECRET:
        token_data["client_secret"] = CLIENT_SECRET

    response = requests.post(
        f"https://login.microsoftonline.com/{TENANT}/oauth2/v2.0/token",
        data=token_data,
        timeout=30,
    )
    if response.status_code >= 400:
        raise RuntimeError(f"Microsoft token request failed: {response.status_code} {response.text}")
    tokens = response.json()
    if not tokens.get("access_token") or not tokens.get("refresh_token"):
        raise RuntimeError(f"Token response missing expected fields: {tokens}")
    return tokens


def save_tokens(tokens: dict) -> None:
    Path(OUTPUT_FILE).write_text(json.dumps(tokens, indent=2, ensure_ascii=False), encoding="utf-8")
    print(f"Tokens written to {OUTPUT_FILE}")
    print()
    print("Add these values to .env:")
    print(f"GRAPH_CLIENT_ID={CLIENT_ID}")
    if CLIENT_SECRET:
        print(f"GRAPH_CLIENT_SECRET={CLIENT_SECRET}")
    print(f"GRAPH_ACCESS_TOKEN={tokens['access_token']}")
    print(f"GRAPH_REFRESH_TOKEN={tokens['refresh_token']}")


def main() -> int:
    if not CLIENT_ID:
        raise RuntimeError("CLIENT_ID is required")
    code, code_verifier = authenticate()
    tokens = exchange_code(code, code_verifier)
    save_tokens(tokens)
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (NoSuchElementException, TimeoutException) as exc:
        print(f"ERROR: Microsoft sign-in page automation failed: {exc}", file=sys.stderr)
        raise SystemExit(1)
    except Exception as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        raise SystemExit(1)
