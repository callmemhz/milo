# Milo Apps Kit v1 — Deployment Runbook

This directory contains the files needed to run the Milo Apps Kit control plane on a single VPS.

---

## Prerequisites

- VPS running Linux with Docker Engine (≥24) and the Compose plugin (`docker compose`)
- DNS records on your domain:
  - `*.app.example.com` A → host public IP  (wildcard for user apps)
  - `milo-apps-kit.example.com`  A → host public IP  (API endpoint)
- Ports 80 and 443 open in the host firewall

---

## Bring Up

```bash
cd deploy/
cp env.example .env
# Edit .env — fill in ROOT_DOMAIN, API_DOMAIN, GHCR_USER, GHCR_TOKEN, DNS_API_TOKEN
docker compose up -d
```

On first boot the server prints a one-time bootstrap admin token. Capture it:

```bash
docker compose logs -f milo-apps-kit-control-plane | grep BOOTSTRAP_ADMIN_TOKEN
```

The token is printed exactly once. If you miss it, delete `milo-apps-kit-state` and restart.

---

## First Login (from a workstation)

Install the `milo-apps-kit` CLI (build from source: `go build ./cmd/milo-apps-kit` in this repo, or pull a release binary).

```bash
milo-apps-kit auth login \
  --endpoint=https://milo-apps-kit.example.com \
  --token=<bootstrap-token> \
  --context-name=prod

milo-apps-kit auth whoami   # should return the admin user
```

---

## Create Real Users

```bash
milo-apps-kit users create alice
```

The command returns a plaintext bearer token. Deliver it to alice out-of-band; it is not stored in plaintext and cannot be retrieved later.

To rotate the bootstrap admin credential: create a new admin user, log in as that user, then delete the original admin:

```bash
milo-apps-kit users create new-admin
milo-apps-kit auth login --endpoint=... --token=<new-admin-token> --context-name=prod
milo-apps-kit users delete admin
```

---

## App Lifecycle (alice's perspective)

```bash
# Create an app record
milo-apps-kit apps create myapp --port=8080

# Create a deploy token for CI
milo-apps-kit tokens create myapp
# → copy the token into your CI secret as MILO_APPS_KIT_DEPLOY_TOKEN
```

In your GitHub Actions workflow, after `docker/build-push-action@v5`:

```yaml
- name: Deploy to Milo Apps Kit
  run: |
    curl -fsS -X POST https://milo-apps-kit.example.com/v1/apps/myapp/deployments \
      -H "Authorization: Bearer ${{ secrets.MILO_APPS_KIT_DEPLOY_TOKEN }}" \
      -d '{"image":"ghcr.io/${{ github.repository }}@${{ steps.push.outputs.digest }}"}'
```

The `steps.push.outputs.digest` value is the immutable image digest emitted by `docker/build-push-action@v5`.

The app is then reachable at `https://myapp.app.example.com/`.

---

## Installing the CLI

End users install the CLI from Homebrew or GitHub Releases — the PaaS does
not vend its own CLI binaries.

```bash
# macOS / Linuxbrew
brew install callmemhz/milo-apps-kit/milo-apps-kit

# Or download a release tarball directly:
#   https://github.com/callmemhz/milo-apps-kit/releases/latest
# pick the matching milo-apps-kit_*_<os>_<arch>.tar.gz
```

CLI versions are published on each `git tag vX.Y.Z` push to the source repo
(see `.github/workflows/release.yml`).

---

## Backup

**SQLite database:**

```bash
docker run --rm \
  -v milo-apps-kit-state:/state \
  alpine \
  sqlite3 /state/milo-apps-kit.db .backup /state/backup.db
```

Copy `backup.db` off the host, or simply copy `milo-apps-kit.db` directly if the server is stopped.

**Persistent app data volumes:**

```bash
docker run --rm \
  -v milo-apps-kit-app-myapp-data:/data \
  -v $(pwd):/backup \
  alpine \
  tar czf /backup/myapp-data.tgz /data
```

There is no automatic backup scheduler in v1; set up a cron job externally.

---

## Upgrade

```bash
# Edit .env: bump MILO_APPS_KIT_VERSION to the new tag
docker compose pull
docker compose up -d
```

On restart, the server runs schema migrations and startup hygiene automatically. User app containers (managed outside Compose) are not affected by a control-plane restart.

---

## Known v1 Limitations

- **Single host**: the control plane and data plane share one machine. A control-plane outage means no new deployments and no routing updates; running apps keep serving traffic as long as Caddy is up, but the API is unavailable.
- **All user traffic is funnelled through this one machine**: no horizontal scaling or load balancing.
- **No automatic backup scheduler**: backups must be scripted externally.

---

## Caddy DNS Plugin Variants

The `docker-compose.yml` uses `caddybuilds/caddy-cloudflare:latest`, which bundles the Cloudflare DNS-01 plugin. The base `caddy:2` image does **not** include any DNS provider plugin, so do not swap to it without rebuilding.

If your DNS is not on Cloudflare, change both files:

| Provider   | Compose image                     | Caddyfile directive           |
|------------|-----------------------------------|-------------------------------|
| Cloudflare | `caddybuilds/caddy-cloudflare`    | `dns cloudflare {env.DNS_API_TOKEN}` |
| Route 53   | build with `caddy-dns/route53`    | `dns route53 {env.DNS_API_TOKEN}`   |
| AliDNS     | build with `caddy-dns/alidns`     | `dns alidns {env.DNS_API_TOKEN}`    |

Community-built images are listed at <https://github.com/caddyserver/caddy-docker>.

---

## Caddyfile Notes

- Replace `admin@example.com` with a real address for ACME certificate notifications.
- `labels.3` (zero-indexed) extracts the leftmost subdomain when the root is `app.example.com` (labels: `com`=0, `example`=1, `app`=2, `<app-name>`=3). If you change the root domain depth, update this index accordingly.
- The Caddyfile is mounted read-only (`ro`) into the Caddy container and is never modified at runtime.
