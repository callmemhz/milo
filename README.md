# milo-apps-kit

A lightweight, single-host PaaS for small teams. Inspired by Dokku but smaller
in scope: it does not build images itself, does not manage multiple nodes
(yet), and exposes a REST API + CLI rather than a `git push` flow. Apps are
single Docker containers fronted by Caddy with wildcard subdomain routing.

CI builds the image and pushes to GHCR (or any registry); CI then calls one
HTTP endpoint to deploy. The PaaS pulls the image, runs it on a stable
Docker network alias, health-checks, and rolls traffic over without touching
Caddy config.

## Highlights

- **One HTTP endpoint to deploy:** `POST /v1/apps/{app}/deployments` with an
  image digest. CI is the only deploy trigger.
- **Static Caddy config:** apps are routed via Docker DNS aliases on a single
  shared bridge network. App lifecycle (create / deploy / delete) never
  reloads the proxy.
- **Rolling deploys with zero downtime:** stable network alias + Caddy
  `dynamic a` with `refresh 1s`.
- **Bearer token auth:** user tokens, per-app deploy tokens; admins manage
  users; owners manage their own apps.
- **One-shot pull credentials:** `POST /deployments` accepts an optional
  `registry_auth` field. The server uses those credentials to pull this one
  image and forgets them — no long-lived registry secret on the server.
- **SQLite for state.** Single-host, single binary.

## Requirements

- Linux host with Docker
- A wildcard DNS record (`*.app.example.com → host IP`) for app subdomains
- A second DNS record for the API (`<api-host>.example.com`)
- For wildcard TLS in production: a DNS provider supported by Caddy's DNS-01
  plugin (e.g., Cloudflare, Route53, AliDNS). For local testing, HTTP only is
  fine — see `deploy/Caddyfile.local`.

## Quick start (single host)

```bash
git clone https://github.com/callmemhz/milo-apps-kit.git
cd milo-apps-kit/deploy
cp env.example .env       # fill in ROOT_DOMAIN, API_DOMAIN, etc.
docker compose up -d
docker compose logs milo-apps-kit-control-plane | grep BOOTSTRAP_ADMIN_TOKEN
```

Capture the bootstrap admin token (printed once on stderr) and use it to log
in from your workstation. Install the CLI:

```bash
brew install callmemhz/milo-apps-kit/milo-apps-kit
# or download a binary from https://github.com/callmemhz/milo-apps-kit/releases
```

Then log in:

```bash
milo-apps-kit auth login \
  --endpoint=https://<api-host>.example.com \
  --token=<bootstrap-token>
```

Then create real users, apps, deploy tokens, and deploy.

## Container contract

Your app **must read the `PORT` environment variable** and listen on that
port. The platform always sets `PORT=8080` and Caddy routes `<app>.<root>`
traffic to the container's port 8080. Hardcoding a port in your image
breaks routing. Most modern frameworks already follow this convention
(Express, FastAPI, Spring Boot, Gin, Rails, Next.js, …).

This is the same contract Heroku, Cloud Run, Railway, Render, and Fly use,
so an app that runs on any of those typically runs here unchanged.

## Deploy contract

A typical CI step (GitHub Actions example):

```yaml
- uses: docker/build-push-action@v5
  id: push
  with:
    push: true
    tags: ghcr.io/${{ github.repository }}:${{ github.sha }}

- run: |
    curl -fsS -X POST \
      https://<api-host>.example.com/v1/apps/myapp/deployments \
      -H "Authorization: Bearer ${{ secrets.MILO_APPS_KIT_DEPLOY_TOKEN }}" \
      -d '{"image":"ghcr.io/${{ github.repository }}@${{ steps.push.outputs.digest }}"}'
```

The PaaS pulls by digest (reproducible), runs the container with the app's
configured env / cpu / memory / volume, health-checks, and swaps traffic.
Failed deploys leave the previous container untouched.

## Layout

```
cmd/
  milo-apps-kit/         CLI client (cobra)
  milo-apps-kit-server/  control plane daemon
internal/
  auth/                  bearer middleware + scope guards
  bootstrap/             first-run admin token
  cli/                   cobra commands, HTTP client, output formatter
  config/                envconfig
  deploy/                orchestrator + per-app cancellable lock + hygiene
  docker/                Docker SDK wrapper
  server/                chi router + handlers
  store/                 SQLite via sqlc
pkg/api/                 shared request/response types
deploy/                  Dockerfile, docker-compose.yml, Caddyfile, env.example
migrations/              golang-migrate SQL
```

## Status & scope

This is a v1. Things that are **in**:

- Single host
- Multi-user, multi-owner apps; admin-only delete
- Per-app env, CPU/memory limits, `/data` volume, health check, restart policy
- Rolling deploy with last-write-wins concurrency
- Auto subdomain (`{app}.app.example.com`) with wildcard cert (DNS-01)

Things deliberately **out** of v1 (roadmap):

- Multi-node / SSH agent
- Internal builders / `git push` deploy (CI is the only trigger)
- Custom per-app domains
- Replicas / horizontal scaling
- Per-app network isolation + service `link`
- Managed services (`postgres`, `redis`, …)
- `apps exec` for shell access
- OIDC / SSO

## License

Apache-2.0. See [LICENSE](LICENSE).
