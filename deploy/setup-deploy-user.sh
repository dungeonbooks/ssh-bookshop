#!/usr/bin/env bash
# One-time setup on the shop VM for deploys from CI, and the way to update the
# deploy script afterwards. Idempotent. Run as root from a directory holding
# this file and ssh-bookshop-deploy, with the CI key's public half:
#
#   sudo bash setup-deploy-user.sh 'ssh-ed25519 AAAA... ssh-bookshop-ci'
#
# The deploy user gets no shell and no forwarding: its only key is pinned to
# running the deploy script, which is all its sudo rule allows too.
set -euo pipefail
cd "$(dirname "$0")"

pubkey=${1:?usage: setup-deploy-user.sh '<ssh public key>'}
# One line only: a second line would land in authorized_keys as its own entry,
# without the forced command.
[[ "$pubkey" != *[$'\r\n']* ]] &&
  [[ "$pubkey" =~ ^ssh-ed25519\ [A-Za-z0-9+/=]+(\ [[:print:]]*)?$ ]] || {
  echo "expected one ssh-ed25519 public key" >&2
  exit 1
}

install -m 0755 -o root -g root ssh-bookshop-deploy /usr/local/sbin/ssh-bookshop-deploy

if ! id deploy >/dev/null 2>&1; then
  useradd --system --create-home --home-dir /var/lib/deploy --shell /bin/sh deploy
fi
# '*' rather than the '!' useradd leaves: no password either way, but sshd
# treats '!' as a locked account and refuses its keys too.
usermod -p '*' deploy

install -d -m 0700 -o deploy -g deploy /var/lib/deploy/.ssh
printf 'restrict,command="sudo -n /usr/local/sbin/ssh-bookshop-deploy" %s\n' "$pubkey" \
  >/var/lib/deploy/.ssh/authorized_keys
chown deploy:deploy /var/lib/deploy/.ssh/authorized_keys
chmod 0600 /var/lib/deploy/.ssh/authorized_keys

rule=/etc/sudoers.d/ssh-bookshop-deploy
printf 'deploy ALL=(root) NOPASSWD: /usr/local/sbin/ssh-bookshop-deploy\n' >"$rule.tmp"
chmod 0440 "$rule.tmp"
visudo -cf "$rule.tmp" >/dev/null
mv "$rule.tmp" "$rule"

echo "deploy user ready; the script is $(sha256sum /usr/local/sbin/ssh-bookshop-deploy | cut -c1-12)"
