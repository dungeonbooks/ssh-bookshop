# Deploying the shop

The shop wants to be reachable as `ssh shop.dungeonbooks.com` with no port flag
and no client-side software, because that one-liner is the whole product. That
rules out most managed hosting: it needs a box with a public IPv4 where we
control port 22.

Target is a dedicated small VM, separate from anything else we run, so a
compromise of a public-facing service cannot reach the rest of the tenancy.

## Why not the obvious alternatives

**Railway.** Its TCP proxy hands out a random high port on `*.proxy.rlwy.net`.
Putting a real hostname on port 22 in front of that needs Cloudflare Spectrum,
and Spectrum for arbitrary TCP is Enterprise-only. Not a configuration problem,
a plan boundary.

**Cloudflare in front of the apex.** `dungeonbooks.com` is orange-clouded and
serves the web storefront. A proxied record only carries HTTP(S), so SSH to it
never reaches an origin, and un-proxying the apex would take the storefront with
it. Hence a separate `shop.` name on a grey-clouded (DNS-only) record.

Cloudflare Tunnel does not rescue this either: SSH over a tunnel needs
`cloudflared` on the client, which destroys the one-liner.

**Anything at home.** Publishing a residential IP and accepting inbound
connections onto a home LAN, with CGNAT, dynamic addressing and home-grade
uptime on top.

## DNS

One A record, `shop.dungeonbooks.com` → the VM's public IPv4, **grey cloud /
DNS-only**. Proxying it would break SSH.

That record exposes the origin IP with no Cloudflare protection in front, which
is why the rate limiter matters and why nothing else of ours runs on that box.

## Ports

The shop takes port 22 so visitors need no `-p`. Admin sshd moves off it, and
the tidiest place to put it is the tailnet, which takes our own access off the
public internet entirely.

**`ListenAddress` in `sshd_config` does nothing on Ubuntu 24.04.** It ships with
`ssh.socket` enabled, so systemd owns the listening socket and sshd never reads
that directive. Configure the socket instead:

```sh
# /etc/systemd/system/ssh.socket.d/tailnet-only.conf
[Socket]
# Clear the inherited 0.0.0.0:22 first; ListenStream is additive.
ListenStream=
ListenStream=<tailscale-ip>:22
```

```sh
sudo systemctl daemon-reload && sudo systemctl restart ssh.socket
```

Arm a revert before making the change, so a mistake undoes itself instead of
locking everyone out, and cancel it once a **new** connection over the tailnet
is confirmed working:

```sh
sudo systemd-run --on-active=300 --unit=sshd-revert /bin/sh -c \
  "rm -rf /etc/systemd/system/ssh.socket.d && systemctl daemon-reload && systemctl restart ssh.socket"
# ... verify a fresh tailnet ssh works, then:
sudo systemctl stop sshd-revert.timer
```

## The shop cannot bind 0.0.0.0:22

Once sshd holds `<tailscale-ip>:22`, binding `0.0.0.0:22` collides with it and
the shop dies with `address already in use`. Bind the VNIC's private address
instead. OCI NATs the public IP to it, so visitors still reach the shop on 22
while admin ssh keeps 22 on the tailnet.

That address is per-box, so it goes in the environment file with the other
per-box settings:

```sh
# /etc/ssh-bookshop.env
HOST=<vnic-private-ip>
```

The unit deliberately leaves `HOST` unset. `Environment=` is applied after
`EnvironmentFile=`, so a value in the unit would silently win over the file and
put the collision back.

Two firewall layers, and forgetting the second is the classic "port is open but
it times out": the cloud provider's security list or NSG, and the instance's own
iptables, which most images ship locked down.

```sh
sudo iptables -I INPUT 1 -p tcp --dport 22 -j ACCEPT
sudo iptables -I INPUT 1 -p tcp --dport 80 -j ACCEPT    # ACME HTTP-01 and the redirect
sudo iptables -I INPUT 1 -p tcp --dport 443 -j ACCEPT   # the landing page and the API
sudo netfilter-persistent save
```

`provision-oci.sh` opens the same three in the network security group.

## The host key is permanent

The moment anyone connects, our fingerprint is in their `known_hosts`. Changing
it later greets every returning visitor with `REMOTE HOST IDENTIFICATION HAS
CHANGED`, which reads as a compromised shop.

Generate it once and keep it forever:

```sh
ssh-keygen -t ed25519 -N '' -f hostkey -C 'dungeonbooks shop'
# into /etc/ssh-bookshop.env as a single line
printf 'SSH_HOST_KEY=%s\n' "$(base64 -w0 < hostkey)"
```

`hostKeyPEM` accepts raw PEM, base64 of that PEM, or PEM with escaped newlines.
Base64 is the one to use in an env file, which cannot hold real newlines.

Back the private key up somewhere that is not the VM. Losing it means every
existing visitor gets the mismatch warning.

Publish the fingerprint on the website, since we are asking strangers to trust
on first use and they deserve something to check against:

```sh
ssh-keygen -lf hostkey.pub
```

## The API host

`api.dungeonbooks.com` is a second A record to the same VM, **orange-clouded**.
SSH is why `shop.` cannot be proxied; HTTP has no such problem, and the API is
the one thing here that benefits from Cloudflare in front of it: an
unauthenticated endpoint that creates orders on Square. The Caddyfile lists
Cloudflare's ranges as trusted proxies so the API rate-limits on the visitor's
address rather than the edge's.

The Go process serves the API on `127.0.0.1:8080` (`API_ADDR`; empty turns it
off). Caddy reverse-proxies to it and sets `X-Client-IP`, which the API trusts
because nothing but Caddy can reach the port. Open 80 and 443 in both firewall
layers, same as for the landing page.

Agent-facing files under `deploy/site/` (`llms.txt`, `llms-full.txt`,
`skill.md`, `openapi.json`, `docs/`) ship with the landing page to
`/var/www/shop`. `llms-full.txt` is generated; after editing a page under
`docs/` regenerate it with `scripts/llms-full.sh` and the test suite will hold
you to it. The pages under `docs/` also publish on docs.dungeonbooks.com under
`/agents/`, copied from this directory at that site's build; a change here is
live there on its next build, daily or by hand.

Check before announcing:

```sh
curl -s https://api.dungeonbooks.com/v1/books | head
curl -sI https://shop.dungeonbooks.com/llms.txt | grep -i '^link'
```

## Install

```sh
sudo useradd --system --home /opt/ssh-bookshop --shell /usr/sbin/nologin bookshop
sudo install -d -o bookshop -g bookshop /opt/ssh-bookshop
sudo install -o bookshop -g bookshop ssh-bookshop /opt/ssh-bookshop/ssh-bookshop

sudo install -m 0400 -o root -g root ssh-bookshop.env /etc/ssh-bookshop.env
sudo cp ssh-bookshop.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now ssh-bookshop
```

`/etc/ssh-bookshop.env` holds `SSH_HOST_KEY`, `HOST`, `SQUARE_ACCESS_TOKEN`,
`SQUARE_LOCATION_ID`, and `SQUARE_ENVIRONMENT=production`.

The binary is static (`CGO_ENABLED=0`), so cross-compiling from anywhere works.
For an ARM instance:

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags='-s -w' .
```

## Check before announcing

```sh
ssh -o StrictHostKeyChecking=accept-new shop.dungeonbooks.com   # no -p
ssh-keyscan shop.dungeonbooks.com                               # matches the published fingerprint
```

Confirm prices loaded rather than the shop falling back to browse-only:
`journalctl -u ssh-bookshop | grep 'square ready'`.
