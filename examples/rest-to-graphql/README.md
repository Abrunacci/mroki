# REST → GraphQL Example

Compare a legacy REST service against the GraphQL service that replaces it. mroki-proxy translates
each REST request into a GraphQL query for the shadow service, brings the GraphQL response back to
the REST shape using a field mapping, and prints the differences to the console.

```
client ── GET /bookings/1042 ──► mroki-proxy ──► legacy REST (live, :9001)
                                      │
                                      └── POST /graphql {query, variables: {id: "1042"}}
                                          ──► GraphQL (shadow, :9002)
```

The two services in this directory return the same booking, with two deliberate differences:

| Field | Legacy REST | New GraphQL |
|---|---|---|
| `id` | `1042` (number) | `"1042"` (string, GraphQL `ID`) |
| `check_in` / `checkIn` | `"2026-10-09"` | `"2026-10-09T00:00:00Z"` |

Everything else matches once field names are mapped (`guest_name` ↔ `guestName`, …). The legacy-only
`legacy_code` field is not in the mapping, so it is left out of the comparison.

## Files

- [`mapping.yaml`](mapping.yaml): the REST → GraphQL mapping (route, query, variables, field names)
- [`legacy/main.go`](legacy/main.go): stand-in for the legacy REST service
- [`graphql/main.go`](graphql/main.go): stand-in for the new GraphQL service (answers only this example's query)

## Run It From Scratch (Ubuntu on WSL)

These steps start from a fresh Ubuntu on WSL with no Go installed. Every command can be copied and
pasted as is.

**1. Install Go 1.26.1** (once). This uses the official tarball for x86-64; on an ARM machine replace
`amd64` with `arm64`.

```bash
sudo apt-get update && sudo apt-get install -y git curl ca-certificates
curl -fsSLO https://go.dev/dl/go1.26.1.linux-amd64.tar.gz
sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf go1.26.1.linux-amd64.tar.gz
rm go1.26.1.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
go version
```

`go version` must print `go version go1.26.1 linux/amd64`.

**2. Clone the repository** (once):

```bash
cd ~
git clone https://github.com/Abrunacci/mroki.git
cd ~/mroki
go mod download
```

**3. Start the three processes**, each in its own terminal (in Windows Terminal, open a new Ubuntu
tab for each one). Leave them running.

Terminal 1, legacy REST service (live):

```bash
cd ~/mroki && go run ./examples/rest-to-graphql/legacy
```

It prints `legacy REST bookings service listening on :9001`.

Terminal 2, new GraphQL service (shadow):

```bash
cd ~/mroki && go run ./examples/rest-to-graphql/graphql
```

It prints `GraphQL bookings service listening on :9002`.

Terminal 3, mroki-proxy:

```bash
cd ~/mroki && \
MROKI_APP_LIVE_URL=http://localhost:9001 \
MROKI_APP_SHADOW_URL=http://localhost:9002 \
MROKI_APP_GRAPHQL_CONFIG=examples/rest-to-graphql/mapping.yaml \
go run ./cmd/mroki-proxy
```

It is ready when it prints `graphql shadow adapter configured` and `proxy server started address=:8080`.
The first `go run` of each process compiles it, so it takes a little longer.

**4. Request the booking** from a fourth terminal:

```bash
curl http://localhost:8080/bookings/1042
```

The response is the legacy one, unchanged:

```json
{"id":1042,"guest_name":"Ana Pérez","check_in":"2026-10-09","check_out":"2026-10-12","status":"confirmed","total_price":450.5,"legacy_code":"BK-1042"}
```

**5. Check terminal 3.** The proxy logs the GraphQL call, the REST call and the diff with the two
deliberate differences (timestamps and request IDs will differ):

```
time=2026-10-08T03:00:59.219Z level=DEBUG msg="forwarding request" method=POST url=http://localhost:9002/graphql
time=2026-10-08T03:00:59.219Z level=DEBUG msg="forwarding request" method=GET url=http://localhost:9001/bookings/1042
time=2026-10-08T03:00:59.221Z level=INFO msg="response diff detected" request.id=0a5558ec-944a-48f2-a06d-566b30078fc8 request.method=GET request.path=/bookings/1042 live_status=200 shadow_status=200 changes=2 diff="  replace /body/check_in: \"2026-10-09T00:00:00Z\"\n  replace /body/id: \"1042\"\n"
```

Stop everything with `Ctrl+C` in each terminal. The sections below explain the output and more cases.

## Run It

From the repository root, in three terminals:

```bash
# 1. Legacy REST service (live)
go run ./examples/rest-to-graphql/legacy

# 2. New GraphQL service (shadow)
go run ./examples/rest-to-graphql/graphql

# 3. mroki-proxy in standalone mode with the mapping
MROKI_APP_LIVE_URL=http://localhost:9001 \
MROKI_APP_SHADOW_URL=http://localhost:9002 \
MROKI_APP_GRAPHQL_CONFIG=examples/rest-to-graphql/mapping.yaml \
go run ./cmd/mroki-proxy
```

Then send a request through the proxy:

```bash
curl http://localhost:8080/bookings/1042
```

The client gets the legacy REST response, unchanged:

```json
{"id":1042,"guest_name":"Ana Pérez","check_in":"2026-10-09","check_out":"2026-10-12","status":"confirmed","total_price":450.5,"legacy_code":"BK-1042"}
```

## What You See in the Console

At startup the proxy confirms the mapping was loaded:

```
level=INFO msg="graphql shadow adapter configured" file=examples/rest-to-graphql/mapping.yaml endpoint=/graphql routes=1
```

For `GET /bookings/1042`, the proxy calls the GraphQL service and logs the two deliberate differences
as JSON Patch operations (the value shown is the shadow's):

```
level=DEBUG msg="forwarding request" method=POST url=http://localhost:9002/graphql
level=DEBUG msg="forwarding request" method=GET url=http://localhost:9001/bookings/1042
level=INFO msg="response diff detected" request.method=GET request.path=/bookings/1042 live_status=200 shadow_status=200 changes=2 diff="  replace /body/check_in: \"2026-10-09T00:00:00Z\"\n  replace /body/id: \"1042\"\n"
```

Status codes and headers are not compared, because GraphQL usually answers `200` even for errors.

Two more cases worth trying:

- `curl http://localhost:8080/bookings/9999`: REST answers `404` with an error body, GraphQL answers
  `{"data":{"booking":null}}`. The normalized shadow body is `null`, so the diff shows `replace /body`.
- `curl http://localhost:8080/guests/7`: no route in the mapping, so the request goes to live only
  (`skipping shadow proxy: rewrite failed` at debug level).

## Limitations

This first version supports path parameters as variables, a single root object, and renamed fields
(including nested ones such as `dates.check_in: stay.checkIn`). Not yet supported: variables from the
query string, headers or body; arrays; GraphQL `errors` mapped to status codes; API mode and the Caddy
module. See [Configuration](../../docs/production/CONFIGURATION.md#graphql-shadow-adapter) for the
mapping reference.
