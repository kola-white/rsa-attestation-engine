# Authentication & Session Contract – v0.4

**System:** Cvera / EVT
**Audience:** Principal auth engineer (Go, TS/JS, Ory Kratos)

---

## 1. Topology & Responsibility Split

### 1.1 Runtime Topology

* **Droplet:** `cvera-api` (DigitalOcean)

* **Docker Compose stack:** `apps/evt-api-go/docker-compose.yml`

  ```yaml
  services:
    db:      postgres:16      # Postgres cluster
    api:     evt-api-go       # Go API for EVT domain (api.cvera.app)
    kratos:  oryd/kratos      # Identity provider (auth.cvera.app)
  ```

* **Postgres:**

  * DB `evt` – owner `postgres`, used by Go API.
  * DB `evt_kratos` – owner `evt-app`, used by Kratos.

* **Domains:**

  * `api.cvera.app` → EVT Go API.
  * `auth.cvera.app` → Kratos public endpoint (behind Nginx).

* **TLS / Nginx:**

  * `auth.cvera.app` terminates TLS via Let’s Encrypt.
  * `auth.cvera.app` proxy → Kratos public on `127.0.0.1:4434`.
  * Health: `https://auth.cvera.app/health/ready` → `{"status":"ok"}`.

* **Hardening:**

  * UFW: SSH + Nginx Full (80/443) only.
  * Fail2ban jails: `sshd`, `nginx-auth-cvera` (watching `auth.cvera.app.error.log` for 401s).

### 1.2 Responsibility Split

* **Kratos (`auth.cvera.app`)**

  * System of record for **identities & credentials**.
  * Owns:

    * Registration
    * Login (email/password; later magic link / OTP)
    * Identity schema (`identity.schema.json`)
    * Sessions

* **EVT Go API (`api.cvera.app`)**

  * Domain API for EVT / employment verification.
  * Does **not** own passwords.
  * Issues **API tokens** (access + refresh) **based on Kratos session**.
  * Enforces authorization for:

    * Case list / detail
    * Evidence upload (presigned URL)
    * Approve / reject, etc.

### 1.3 High-Level Auth Architecture

```mermaid
flowchart LR
    subgraph Client["Expo iOS Client"]
        A[Expo App<br/>React Native / SDK 54]
    end

    subgraph Edge["Ingress / TLS"]
        NAuth[Nginx<br/>auth.cvera.app]
        NApi[Nginx<br/>api.cvera.app]
    end

    subgraph KratosCluster["Identity Provider (Kratos)"]
        KPub[Kratos Public<br/>serve public :4434]
        KAdm[Kratos Admin<br/>serve admin :4435]
        subgraph DBK["Postgres DB\n`evt_kratos`"]
            PK[(identities, credentials,<br/>Kratos sessions)]
        end
    end

    subgraph EvtAPI["EVT Domain API"]
        API[EVT Go API<br/>api.cvera.app]
        subgraph DBA["Postgres DB\n`evt`"]
            PE[(domain users, cases,<br/>refresh_tokens (hashes))]
        end
    end

    A -->|"HTTPS\nemail/password,\nself-service login flows"| NAuth
    NAuth -->|"HTTP :4434"| KPub
    KPub -->|"identities / sessions"| PK

    A -->|"HTTPS\naccess_token (JWT),\nrefresh_token (opaque hex)"| NApi
    NApi -->|"HTTP :8080"| API
    API -->|"SQL\n(domain data,\nrefresh token hashes)"| PE

    API -->|"HTTP :4435\nKratos admin API\n(session / identity lookup)"| KAdm
    KAdm -->|"SQL\n(identity state)"| PK

    classDef client fill:#e3f2fd,stroke:#1e88e5,color:#0d47a1;
    classDef ingress fill:#ede7f6,stroke:#5e35b1,color:#311b92;
    classDef kratos fill:#fff3e0,stroke:#fb8c00,color:#e65100;
    classDef api fill:#e8f5e9,stroke:#43a047,color:#1b5e20;
    classDef db fill:#f5f5f5,stroke:#757575,color:#424242,font-size:11px;

    class Client client;
    class Edge ingress;
    class KratosCluster kratos;
    class EvtAPI api;
    class DBK,DBA db;
```
---

## 2. Identity Model

### 2.1 Identity Store (Kratos)

* Kratos uses DB `evt_kratos` with identity schema:

  * Primary identifier: `email`
  * Attributes: `name`, org info, etc. (defined in `identity.schema.json`)
  * Authentication factors: email + password (Phase 1).

* Kratos is the **only** component that stores/validates passwords.

### 2.2 Domain User View (EVT API)

The EVT API treats a user as:

```ts
type Role = "hr_reviewer" | "admin" | "auditor"; // Phase 1: "hr_reviewer"

interface DomainUser {
  id: string;      // Kratos identity ID or mapped UUID
  email: string;
  name: string;
  role: Role;
  status: "active" | "locked" | "disabled";
}
```

Mapping from Kratos → `DomainUser` is either:

* Direct (fields in identity schema), or
* Via a small mapping table in `evt` db keyed by Kratos identity ID.

---

## 3. Session & Token Model (API)

Kratos handles **login / registration / credential lifecycle**.
The EVT API handles **short-lived API sessions** via tokens.

### 3.1 Token Types (unchanged from v0.3)

1. **Access Token (API)**

   * Type: **JWT**
   * Lifetime: **15 minutes**
   * Transport: `Authorization: Bearer <access_token>`
   * Storage: in-memory only (client)
   * Use: authorize API calls.

2. **Refresh Token (API)**

   * Type: **opaque random token** (256-bit, lowercase hex string)
   * Lifetime: **30 days**
   * Transport: JSON body (`refresh_token`)
   * Storage (client): SecureStore (Expo iOS)
   * Storage (server): **SHA-256 hash of token** in DB (`evt.refresh_tokens`)
   * Use: renew access token; rotate refresh; log out.

Kratos sessions are **upstream**; EVT tokens are **downstream** (resource API).

### 3.2 Access Token Claims

JWT payload example:

```json
{
  "sub": "kratos-identity-id-or-mapped-user-id",
  "email": "hr@example.com",
  "roles": ["hr_reviewer"],
  "typ": "access",
  "iat": 1735400000,
  "exp": 1735400900,
  "iss": "cvera-api",
  "aud": "cvera-app"
}
```

Requirements:

* `typ` MUST be `"access"`.
* `exp` MUST be enforced on every request.
* Signing: HS256 or RS256 (choose and document key management).

### 3.3 Refresh Token Format & Storage

* Raw token:

  * 32 random bytes → hex string (length 64), lowercase.
* Server stores:

  * `token_hash = hex(sha256(refresh_token_hex))`.
* Raw token **never stored** server-side.

Refresh token table in DB `evt`:

```sql
CREATE TABLE refresh_tokens (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token_hash  TEXT NOT NULL,         -- hex(sha256(refresh_token))
    user_id     UUID NOT NULL,         -- FK to mapped domain user table
    device_id   TEXT,
    expires_at  TIMESTAMPTZ NOT NULL,
    revoked     BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_refresh_tokens_token_hash ON refresh_tokens(token_hash);
CREATE INDEX idx_refresh_tokens_user_id   ON refresh_tokens(user_id);
```

Validation rules:

* Valid if:

  * Record exists.
  * `revoked = FALSE`.
  * `expires_at > now()`.
  * User status is `active`.
  * Optional: `device_id` matches.

---

## 4. Kratos Integration Contract

### 4.1 Kratos Service Layout

* Public base URL: `https://auth.cvera.app`

  * Terminates TLS.
  * Proxies to Kratos public serve on `127.0.0.1:4434`.
* Admin base URL: `http://kratos:4435` (Docker network) or `http://127.0.0.1:4435` inside droplet (for API calls).
* Kratos config:

  * `serve.public.base_url: https://auth.cvera.app`
  * Identity schema: `file:///etc/config/kratos/identity.schema.json`.

### 4.2 Auth Flow Responsibility

* **Expo iOS app → Kratos (auth.cvera.app)**:

  * Handles:

    * Self-service login flow (email/password).
    * Password reset / registration in future.
  * Output: a **Kratos session** (session cookie or token, depending on chosen mode).

* **Expo iOS app → EVT API (api.cvera.app)**:

  * After Kratos login, client calls EVT API to get **API tokens**.

### 4.3 Proposed Integration Pattern

To keep roles clean:

1. **Kratos handles authentication.**
2. **EVT API consumes a Kratos session** and mints API tokens.

Concretely, introduce:

* `POST /auth/exchange` on `api.cvera.app`:

  * Input: a Kratos session token or ID (the exact shape to be chosen by principal engineer; options include Kratos session cookie, session token from the native API, or a “whoami” call).

  * Process:

    1. EVT API calls Kratos **admin** endpoint to validate the session and retrieve identity.
    2. EVT API maps Kratos identity to `DomainUser` (role, status).
    3. EVT API issues:

       * Access token (JWT)
       * Refresh token (opaque, hex)
    4. Stores refresh token hash in `evt.refresh_tokens`.

  * Output: same JSON as `/auth/login` in v0.3 (minus password).

> **Note:** v0.3 `/auth/login` (email/password) should be considered **deprecated** in favor of `POST /auth/exchange` that uses Kratos as the IDP.

### 4.4 Auth Flow (Sequence): Kratos -> /auth/exchange -> JWT + Refresh

sequenceDiagram
    autonumber

    participant U as User (HR)
    participant C as Expo App<br/>iOS (SDK 54)
    participant KP as Kratos Public<br/>auth.cvera.app
    participant KA as Kratos Admin<br/>kratos:4435
    participant A as EVT Go API<br/>api.cvera.app
    participant D as EVT DB<br/>evt.refresh_tokens

    Note over C,KP: Phase 1 login is delegated to Kratos (IDP).

    U->>C: Open app, tap "Sign in"
    C->>KP: HTTPS /self-service/login (email + password)
    KP->>KP: Validate credentials, create session
    KP-->>C: 200 OK + Kratos session (token/cookie)

    Note over C: App now has a valid Kratos session.<br/>Next: exchange for API tokens.

    C->>A: HTTPS POST /auth/exchange<br/>{ kratos_session_token, device_id? }
    A->>KA: HTTP GET /sessions/whoami or /sessions/{id}<br/>(using kratos_session_token)
    KA->>KA: Validate session, load identity
    KA-->>A: Identity payload (id, email, traits...)

    A->>A: Map identity -> DomainUser (role, status)
    A->>A: Check user status == active

    A->>A: Generate access_token (JWT, 15m)
    A->>A: Generate refresh_token (32-byte random hex)
    A->>D: INSERT refresh_tokens<br/>(sha256(refresh_token), user_id, device_id, expires_at)

    D-->>A: OK

    A-->>C: 200 OK<br/>{ access_token, refresh_token, user }

    Note over C: Store refresh_token in SecureStore.<br/>Keep access_token in memory only.

    C-->>U: User is now signed in to EVT API<br/>(Case list, evidence, approve/reject)

---

## 5. EVT API Auth Endpoints (Phase 1)

All endpoints on `api.cvera.app`.

### 5.1 `POST /auth/exchange` – Kratos → EVT API Tokens

**Purpose:** Exchange a valid Kratos session for an EVT access+refresh token pair.

**Request** (example shape – to be finalized once Kratos token strategy is chosen):

```http
POST /auth/exchange
Content-Type: application/json
```

```json
{
  "kratos_session_token": "<opaque-session-or-bearer-from-kratos>",
  "device_id": "optional-device-uuid"
}
```

**Server behavior:**

1. Call Kratos admin API:

   * e.g. `GET /sessions/whoami` or `GET /sessions/{id}` with the provided token.
2. Validate:

   * Session is active.
   * Identity exists.
3. Map identity → `DomainUser` (role, status).
4. If user locked/disabled → `403 account_locked`.
5. Generate:

   * Access token (JWT, 15 min).
   * Refresh token (opaque hex, 30 days).
6. Store refresh token hash in `evt.refresh_tokens`.

**Success Response**

```json
{
  "access_token": "<jwt-access-token>",
  "refresh_token": "819bd37e8dd478a8f3c4945b882be1c0afd75c3edc48281e2a7f6cb4bc3fcd3a",
  "user": {
    "id": "user-uuid-or-kratos-id",
    "email": "hr@example.com",
    "name": "HR Manager Name",
    "role": "hr_reviewer"
  }
}
```

**Errors**

* `401 invalid_token` – Kratos session invalid / expired.
* `403 account_locked` – identity is locked/disabled.
* `400 invalid_request` – missing or malformed session token.

---

### 5.2 `POST /auth/refresh` – Renew EVT Tokens

**Purpose:** Issue a new access token and rotate refresh token, independent of Kratos (as long as user is still valid).

**Request**

```http
POST /auth/refresh
Content-Type: application/json
```

```json
{
  "refresh_token": "819bd37e8dd478a8f3c4945b882be1c0afd75c3edc48281e2a7f6cb4bc3fcd3a",
  "device_id": "optional-device-uuid"
}
```

**Success Response**

```json
{
  "access_token": "<new-jwt-access-token>",
  "refresh_token": "3ab46c8ae41a7b8253e3b0c7fce9e1d783c9b5fcb7b6f5a69d0d096a1c2b6d11"
}
```

**Server behavior (summary):**

* Hash incoming token.
* Lookup refresh token record + user.
* Validate:

  * Not revoked.
  * `expires_at > now()`.
  * User status is `active`.
  * Optional: `device_id` match.
* Rotate token: new random hex token, new hash, new `expires_at`.

---

### 5.3 `POST /auth/logout` – Revoke EVT Refresh Token

**Purpose:** Invalidate a single EVT refresh token (session logout).

**Request**

```json
{
  "refresh_token": "819bd37e8dd478a8f3c4945b882be1c0afd75c3edc48281e2a7f6cb4bc3fcd3a",
  "device_id": "optional-device-uuid"
}
```

**Behavior:**

* Hash the token.
* Mark `revoked = TRUE` for the matching record (if exists).
* Client clears local storage (SecureStore) regardless of server response.

---

### 5.4 `GET /auth/me` – Resolve Current User

**Purpose:** Resolve `DomainUser` from access token.

**Request**

```http
GET /auth/me
Authorization: Bearer <access_token>
```

**Response**

```json
{
  "user": {
    "id": "user-uuid-or-kratos-id",
    "email": "hr@example.com",
    "name": "HR Manager Name",
    "role": "hr_reviewer"
  }
}
```

---

## 6. Client Flows (Expo iOS)

### 6.1 High-Level Lifecycle

1. **Login (Kratos)**

   * App calls Kratos public endpoints (`auth.cvera.app`) to perform login (email/password UI, Kratos-native React Native flow).
   * On success, client receives a **Kratos session token** (exact pattern TBD: cookie, session token header, or explicit API token).

2. **Session Exchange**

   * App calls `POST https://api.cvera.app/auth/exchange` with Kratos session token.
   * Receives:

     * EVT `access_token` (JWT, 15 min)
     * EVT `refresh_token` (opaque hex, 30 days)
     * `user` object

3. **Normal API Calls**

   * App attaches `Authorization: Bearer <access_token>` to EVT API calls.

4. **Refresh**

   * On access token expiry or 401:

     * App calls `/auth/refresh` with EVT refresh token.
     * Receives new access+refresh.

5. **Logout**

   * App calls `/auth/logout` with EVT refresh token, clears SecureStore, resets auth state.

6. **Face ID**

   * Acts as a **local gate** before using the stored EVT refresh token.
   * Kratos is not aware of biometrics; it remains a client-only concern.

---

## 7. Security Controls (Summary)

* Kratos:

  * Uses its own secrets (`KRATOS_SECRET_DEFAULT`, `KRATOS_SECRET_COOKIE`) and runs in Docker as `kratos` service.
  * Exposed only via Nginx `auth.cvera.app` over HTTPS.
* EVT API:

  * Uses its own JWT signing key (`JWTSigningKey`) and DB `evt`.
  * Guards all protected endpoints with JWT verification + RBAC.
* Network / OS:

  * UFW only allows SSH + Nginx Full (80/443).
  * Fail2ban jails on SSH + `auth.cvera.app` 401 patterns.
* DB:

  * Kratos and EVT use separate databases (`evt_kratos`, `evt`) and app roles (`evt-app`).
  * Refresh tokens are stored as **hashes** only, not raw values.

