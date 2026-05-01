# MCP Dispatch Runbook Template

**Last verified:** 2026-05-01

Use this template to create a consumer-specific live MCP runbook. Replace placeholders with the server's actual URLs, auth credentials, expected surface counts, and evidence paths.

## Environment

```bash
MCP_BASE_URL="http://localhost:PORT"
MCP_ORIGIN="http://localhost:PORT"
CYCLE_ID="cycle-$(date +%Y-%m-%d)"
EVIDENCE_DIR="docs/plans/mcp/evidence/$CYCLE_ID"
mkdir -p "$EVIDENCE_DIR"
```

## Pre-Flight

```bash
# Consumer test gate.
<run test command>

# Consumer lint gate.
<run lint command>

# Health endpoint.
curl -fsS "$MCP_BASE_URL/<health-path>" | jq .

# OAuth metadata.
curl -fsS "$MCP_BASE_URL/.well-known/oauth-protected-resource" | jq .
curl -fsS "$MCP_BASE_URL/.well-known/oauth-authorization-server" | jq .

# Anonymous MCP request should be rejected when auth is enabled.
curl -i -X POST "$MCP_BASE_URL/mcp" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize"}'
```

## Phase 0 - Bootstrap OAuth + MCP Session

### Register Client

```bash
REG=$(curl -fsS -X POST "$MCP_BASE_URL/mcp-oauth/register" \
  -H 'Content-Type: application/json' \
  -d '{"client_name":"e2e-'"$CYCLE_ID"'","redirect_uris":["http://localhost:9999/cb"],"grant_types":["authorization_code","refresh_token"],"response_types":["code"],"token_endpoint_auth_method":"none"}')

echo "$REG" | jq . | tee "$EVIDENCE_DIR/register.json"
CLIENT_ID=$(echo "$REG" | jq -r .client_id)
```

### Generate PKCE Pair

```bash
CODE_VERIFIER=$(openssl rand 96 | openssl base64 -A | tr '+/' '-_' | tr -d '=' | cut -c1-128)
CODE_CHALLENGE=$(echo -n "$CODE_VERIFIER" | openssl dgst -sha256 -binary | openssl base64 -A | tr '+/' '-_' | tr -d '=')
STATE="state-$(openssl rand -hex 8)"
```

### Authorize

Open the authorization URL, complete login/consent, and copy the returned `code`.

```bash
open "$MCP_BASE_URL/mcp-oauth/authorize?response_type=code&client_id=$CLIENT_ID&redirect_uri=http://localhost:9999/cb&code_challenge=$CODE_CHALLENGE&code_challenge_method=S256&state=$STATE&scope=mcp.read+mcp.write+offline_access"
CODE="<paste code>"
```

### Exchange Token

```bash
TOKEN_RESP=$(curl -fsS -X POST "$MCP_BASE_URL/mcp-oauth/token" \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  -d "grant_type=authorization_code&code=$CODE&redirect_uri=http://localhost:9999/cb&client_id=$CLIENT_ID&code_verifier=$CODE_VERIFIER")

echo "$TOKEN_RESP" | jq . | tee "$EVIDENCE_DIR/token.json"
TOKEN=$(echo "$TOKEN_RESP" | jq -r .access_token)
REFRESH=$(echo "$TOKEN_RESP" | jq -r .refresh_token)
```

Do not commit token evidence unless secrets are redacted.

### Initialize MCP Session

```bash
curl -fsSi -X POST "$MCP_BASE_URL/mcp" \
  -H "Origin: $MCP_ORIGIN" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"e2e-bootstrap","version":"0"}}}' \
  > "$EVIDENCE_DIR/initialize.txt"

SESSION=$(grep -i '^mcp-session-id:' "$EVIDENCE_DIR/initialize.txt" | awk '{print $2}' | tr -d '\r')

curl -fsS -X POST "$MCP_BASE_URL/mcp" \
  -H "Origin: $MCP_ORIGIN" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H "Authorization: Bearer $TOKEN" \
  -H "Mcp-Session-Id: $SESSION" \
  -d '{"jsonrpc":"2.0","method":"notifications/initialized"}'
```

### Smoke Surface Inventory

```bash
curl -fsS -X POST "$MCP_BASE_URL/mcp" \
  -H "Origin: $MCP_ORIGIN" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H "Authorization: Bearer $TOKEN" \
  -H "Mcp-Session-Id: $SESSION" \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' \
  > "$EVIDENCE_DIR/tools_list.raw"

grep '^data: ' "$EVIDENCE_DIR/tools_list.raw" | head -1 | sed 's/^data: //' \
  | tee "$EVIDENCE_DIR/tools_list.json" | jq '.result.tools | length'
```

Expected: `<expected tool count>`.

## Phase 1 - Tool Coverage

For each tool:

1. Verify schema appears in `tools/list`.
2. Run one happy-path `tools/call`.
3. Run required-argument and invalid-argument negative paths.
4. Verify JSON-RPC error code and message shape.
5. Save raw and parsed evidence.

## Phase 2 - Resource Coverage

For each resource:

1. Verify it appears in `resources/list`.
2. Read it with expected arguments.
3. Verify content type and payload shape.
4. Verify not-found or unauthorized behavior.

## Phase 3 - Prompt Coverage

For each prompt:

1. Verify it appears in `prompts/list`.
2. Fetch it with required arguments.
3. Verify multi-line content is valid JSON inside the transport frame.
4. Verify missing-argument behavior.

## Phase 4 - Auth + Scope Coverage

Exercise:

- Missing bearer token.
- Invalid bearer token.
- Expired token.
- Token with missing scope.
- PAT path if the consumer supports PATs.
- Disallowed origin.

## Phase 5 - Real-Client Gates

Run the server through MCP Inspector:

```bash
npx @modelcontextprotocol/inspector --cli --transport http --server-url "$MCP_BASE_URL/mcp"
```

Then run at least one MCP-aware coding client and execute one read-only tool end to end.

## Closeout

Before closing:

1. Re-run test and lint gates.
2. Confirm all phases are `DONE` or explicitly `DEFERRED`.
3. Redact secrets from committed evidence.
4. Write cycle summary.
5. Record carryover findings with owner and next action.
