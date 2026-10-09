# REST → GraphQL Example

Compare a legacy REST service against the GraphQL service that replaces it, and see the comparison in
mroki hub. mroki-proxy translates each REST request into a GraphQL query for the shadow service, and
mroki-api brings the GraphQL response back to the REST shape using a field mapping before it diffs
the two, so the hub shows only real differences, not protocol differences.

```
client ── GET /bookings/1042 ──► mroki-proxy ──► legacy REST (live)
                                      │
                                      ├── POST /graphql {query, variables: {id: "1042"}}
                                      │   ──► GraphQL (shadow)
                                      │
                                      └── both responses ──► mroki-api ──► mroki hub
                                          (normalized with the gate's mapping, then diffed)
```

The two services in this directory return the same bookings, with two deliberate differences:

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
- [`compose.yaml`](compose.yaml): the whole stack (Postgres, mroki-api, mroki-proxy, mroki hub and the two services)
- [`setup/main.go`](setup/main.go): creates the gate through the API with `mapping.yaml` as its mapping
- [`traffic/main.go`](traffic/main.go): sends three sample requests through the proxy
- [`Dockerfile`](Dockerfile): one image for the four example programs

## See It in the Hub (5 Minutes)

You need **git** and **Docker** with Compose v2 (`docker compose version` must work). On Windows, use
Docker Desktop with WSL integration turned on for your Ubuntu distribution, and run the commands in
Ubuntu.

**1. Start everything** with one command, from the repository root:

```bash
git clone https://github.com/Abrunacci/mroki.git   # once
cd mroki
docker compose -f examples/rest-to-graphql/compose.yaml up --build
```

The first run builds the images, which takes a few minutes. The stack is ready when the log shows
the sample traffic:

```
traffic-1  | ... GET /bookings/9999 -> 404 {"error":"booking not found"}
traffic-1  | ... sent 3 requests; open the hub to see the comparisons
```

Leave this terminal running. If a port is already in use, see [Ports](#ports) below.

**2. Open the hub at http://localhost:3000.** You see one gate, `bookings-rest-to-graphql`, marked
**REST → GraphQL**, with 3 requests in the last 24 hours.

**3. Open the gate.** Next to its name, the badge shows the mapping version (a short hash of
`mapping.yaml`). Below are the three requests the example sent: `/bookings/1042` and `/bookings/1043`
(both found in both systems) and `/bookings/9999` (found in neither). All three are marked **Diff**.

**4. Open `GET /bookings/1042`.** This is the comparison:

- A note says **REST → GraphQL · body only**: the GraphQL response was normalized to the REST shape
  with the mapping version shown, and only the mapped body fields were compared.
- Live (REST) on the left, shadow (GraphQL, already in the REST shape) on the right.
- Two lines are marked: `id` (`1042` vs `"1042"`) and `check_in` (`"2026-10-09"` vs
  `"2026-10-09T00:00:00Z"`). Field names do not show up as differences, and neither does
  `legacy_code`, which is not in the mapping.
- Live and shadow status codes are shown as **(not compared)**: GraphQL answers `200` even for errors.

**5. Open `GET /bookings/9999`.** REST answered `404` and GraphQL answered `200` with a `null`
booking. The body difference is what is marked: `{}` on the live side (the error body has none of the
mapped fields) against `null` on the shadow side.

**6. Open the gate's Settings.** The **Shadow Adapter** section shows the mapping exactly as in
`mapping.yaml`. It is read-only in the hub; it is changed through the API (step 8).

**7. Send more traffic** from another terminal, then reload the gate:

```bash
curl http://localhost:8080/bookings/1042
```

The client always gets the legacy REST response, unchanged.

**8. Change the mapping and see the versions.** Edit `mapping.yaml`, for example comment out the last
line (`# total_price: totalPrice`), and recreate the setup, the proxy and the traffic:

```bash
docker compose -f examples/rest-to-graphql/compose.yaml up -d --force-recreate gate-setup mroki-proxy traffic
```

The gate keeps its requests and gets the new mapping, so the badge shows a new version. The earlier
requests are marked **Other mapping** in the list, and their detail warns that they were compared
with a different mapping than the gate's current one. Undo the edit and run the command again to go
back.

**9. Stop everything** with `Ctrl+C`, then remove the containers and the data:

```bash
docker compose -f examples/rest-to-graphql/compose.yaml down -v
```

### Ports

The stack publishes these ports on your machine. Each one can be changed with a variable, without
editing any file:

| Service | Default | Variable |
|---|---|---|
| mroki hub | 3000 | `MROKI_HUB_PORT` |
| mroki-proxy | 8080 (admin 8081) | `MROKI_PROXY_PORT`, `MROKI_PROXY_ADMIN_PORT` |
| mroki-api | 8090 | `MROKI_API_PORT` |
| Postgres | 5432 | `MROKI_DB_PORT` |

For example, if a local Postgres already uses port 5432:

```bash
MROKI_DB_PORT=5433 docker compose -f examples/rest-to-graphql/compose.yaml up --build
```

`MROKI_DB_PORT` changes only the port on your machine; inside the stack, mroki-api still reaches
Postgres on 5432. Do not edit `build/package/mroki-db/compose.yaml`. Use the same variables with the
other `docker compose` commands of this guide, so they refer to the same stack.

### What the Setup Does

The `gate-setup` container does what a person would do with the API: it creates the gate with
`mapping.yaml` as its shadow adapter (or, if the gate exists, updates its mapping with
`PATCH /gates/{id}`). The equivalent `curl` call, with the example's API key, is:

```bash
jq -n --rawfile mapping examples/rest-to-graphql/mapping.yaml '{
  name: "bookings-rest-to-graphql",
  live_url: "http://legacy:9001",
  shadow_url: "http://graphql:9002",
  shadow_adapter: {type: "graphql", config: $mapping}
}' | curl -X POST http://localhost:8090/gates \
  -H "Authorization: Bearer mroki-example-api-key-16" \
  -H "Content-Type: application/json" -d @-
```

Run as is against the running example, it answers `409 Conflict`, because the gate already exists.

mroki-proxy then starts in API mode with that gate: it reads the URLs and the mapping from the API,
rewrites each mapped REST request into a GraphQL query for shadow, and sends both raw responses to
mroki-api, which normalizes and diffs them.

## Without Docker: Standalone Proxy, Console Only

The proxy can also run on its own, without mroki-api or the hub. The mapping is then a local file and
the differences are printed in the proxy's log, which is handy to try a mapping quickly.

### From Scratch (Ubuntu on WSL)

These steps start from a fresh Ubuntu on WSL with no Go installed. Every command can be copied and
pasted as is.

**1. Install Go 1.26.1** (once). This uses the official tarball for x86-64; on an ARM machine replace
`amd64` with `arm64`.

```bash
sudo apt-get update && sudo apt-get install -y git curl ca-certificates
curl -fsSLO https://go.dev/dl/go1.26.1.linux-amd64.tar.gz
sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf go1.26.1.linux-amd64.tar.gz
rm go1.26.1.linux-amd64.tar.gz
```

Add Go to the `PATH` in your shell's startup file. Run `echo $SHELL` to see which shell you use, then
run the block for it.

If it prints `/bin/bash` (the Ubuntu default):

```bash
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
go version
```

If it prints `/bin/zsh` or `/usr/bin/zsh`:

```bash
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.zshrc
source ~/.zshrc
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

### Short Version

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

### What You See in the Console

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

This version supports path parameters as variables, a single root object, and renamed fields
(including nested ones such as `dates.check_in: stay.checkIn`). Not yet supported: variables from the
query string, headers or body; arrays; type coercion (so `id` differs on every booking, number vs
string); GraphQL `errors` mapped to status codes; editing the mapping in the hub; and the Caddy
module. See [Configuration](../../docs/production/CONFIGURATION.md#graphql-shadow-adapter) for the
mapping reference.
