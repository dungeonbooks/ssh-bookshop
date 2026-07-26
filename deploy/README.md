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
public internet entirely:

```sh
# /etc/ssh/sshd_config
ListenAddress <tailscale-ip>
Port 22
```

Reconnect over Tailscale and confirm it works **before** freeing the public
socket, or the box becomes unreachable.

Two firewall layers, and forgetting the second is the classic "port is open but
it times out": the cloud provider's security list or NSG, and the instance's own
iptables, which most images ship locked down.

```sh
sudo iptables -I INPUT 1 -p tcp --dport 22 -j ACCEPT
sudo netfilter-persistent save
```

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

`/etc/ssh-bookshop.env` holds `SSH_HOST_KEY`, `SQUARE_ACCESS_TOKEN`,
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
