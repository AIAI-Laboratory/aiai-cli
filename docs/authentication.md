# AIAI Authentication Architecture

AIAI CLI authenticates users via the official [GitHub OAuth Device Flow](https://docs.github.com/en/apps/oauth-apps/building-oauth-apps/authorizing-oauth-apps#device-flow), which is specifically designed for terminal and native applications.

In this architecture, the CLI directly interfaces with GitHub OAuth and REST APIs:
1. It does **not** embed or require any `client_secret` into the compiled binary.
2. It requests only minimal, public profile permissions (`read:user`), without requesting repository or private email access.
3. Credentials are securely stored using OS native keyrings (macOS Keychain, Windows Credential Manager, Linux Secret Service) with a permission-locked `0600` file fallback.
4. When the AIAI Cloud API backend is deployed, an optional exchange step will allow exchanging the verified GitHub identity for an AIAI session token.

---

## Authentication Flow

```
┌──────────┐              ┌──────────────┐              ┌──────────────┐
│ AIAI CLI │              │  GitHub API  │              │ User Browser │
└────┬─────┘              └──────┬───────┘              └──────┬───────┘
     │                           │                             │
     │ 1. POST /login/device/code│                             │
     │    (client_id, read:user) │                             │
     ├──────────────────────────►│                             │
     │                           │                             │
     │ 2. device_code, user_code │                             │
     │◄──────────────────────────┤                             │
     │                                                         │
     │ 3. Open browser & display user_code                     │
     ├────────────────────────────────────────────────────────►│
     │                                                         │
     │ 4. Poll POST /login/oauth/access_token                  │
     │    (grant_type=device_code)                             │
     ├──────────────────────────►│                             │
     │                           │                             │
     │ 5. Returns access_token   │                             │
     │◄──────────────────────────┤                             │
     │                                                         │
     │ 6. GET /user (Bearer token)                             │
     ├──────────────────────────►│                             │
     │                           │                             │
     │ 7. Returns profile info   │                             │
     │◄──────────────────────────┤                             │
     │                                                         │
     │ 8. Save token securely to OS Keyring                    │
     ├─┐                                                       │
     │ │ (macOS Keychain, Windows Credential Manager,          │
     │ │  or Linux Secret Service; fallback 0600 file)         │
     │◄┘                                                       │
     ▼                                                         ▼
```

---

## Configuration

### GitHub OAuth Client ID

To use GitHub Device Flow, create a GitHub OAuth App with Device Flow enabled in **GitHub Developer Settings**:
- Set **Enable Device Flow** to enabled.
- Callback URL is not required for Device Flow (a placeholder such as `http://localhost` can be provided).

Pass the Client ID to `aiai` via:
- **Environment Variable (recommended)**:
  ```bash
  export AIAI_GITHUB_CLIENT_ID="Ov23..."
  # or export GITHUB_CLIENT_ID="Ov23..."
  ```
- **Build time injection**:
  ```bash
  go build -ldflags "-X main.githubClientID=Ov23..." ./cmd/aiai
  ```

---

## Security & Credential Storage

- **No Client Secret**: In accordance with [GitHub OAuth Native Application Guidance](https://docs.github.com/en/apps/oauth-apps/building-oauth-apps/best-practices-for-creating-an-oauth-app), native client binaries must never contain client secrets.
- **Least Privilege**: Only the public user profile scope (`read:user`) is requested.
- **OS Keyring**: Credentials are encrypted and stored in the operating system's native keychain:
  - macOS: Keychain Services
  - Windows: Windows Credential Manager
  - Linux: FreeDesktop Secret Service via DBus
- **File Fallback**: When headless or if keyring services are unavailable, credentials fall back to `~/.config/aiai/credentials.json` (or platform equivalent) strictly enforced with `0600` owner-only permissions and symlink rejection.
- **Output Redaction**: `--json` output and logs sanitize and redact tokens to prevent terminal leakage.
