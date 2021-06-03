#!/usr/bin/env bash

BIN_NAME=booty
EX_CURL="$(command -v 'curl' &>/dev/null && echo 0 || echo 1)"
# TODO: uses gpg to release assets
#EX_GPG="$(
#	command -v 'gpg' &
#	>/dev/null && echo 0 || echo 1
#)"
# shellcheck disable=SC2060
OS="$(uname -s | tr [:upper:] [:lower:])"
OS_ARCH=""
RELEASE_URL="https://github.com/amplify-edge/booty/releases"
LATEST_RELEASE_URL="${RELEASE_URL}/latest"
INSTALL_LOC=/usr/local/bin

# checks if we're running as root
runAsRoot() {
  if [ $EUID -ne 0 ]; then
    sudo "${@}"
  else
    "${@}"
  fi
}

osArch() {
  ARCH="$(uname -m)"
  case "$ARCH" in
  x86_64) ARCH="amd64" ;;
  x86-64) ARCH="amd64" ;;
  aarch64) ARCH="arm64" ;;
  esac
  OS_ARCH="${OS}_${ARCH}"
}

validOsArchCombo() {
  case "$OS_ARCH" in
  linux_amd64) return 0 ;;
  darwin_amd64) return 0 ;;
  darwin_arm64) return 0 ;;
  *) return 1 ;;
  esac
}

latestVersion() {
  curl -sL "${LATEST_RELEASE_URL}" | grep "Release" | head -n 1 | awk '{n=split($2,a,"·");print a[n]}'
}

fetchAndInstall() {
  local tag_format="${BIN_NAME}-${TAG}-${OS_ARCH}.tar.gz"
  # download
  curl -L -o "/tmp/${tag_format}" "${RELEASE_URL}/download/${TAG}/${tag_format}"
  # extract
  tar -C /tmp -zxvf "/tmp/${tag_format}"
  # install
  runAsRoot install -m755 /tmp/${BIN_NAME} "${INSTALL_LOC}/${BIN_NAME}"
  echo "installation complete"
}

if [ ! "$EX_CURL" ]; then
  echo "please install curl first"
  exit 1
fi
osArch
VALID=$(validOsArchCombo)
if [ "${VALID}" ]; then
  echo "unsupported os and arch ${OS_ARCH}"
  exit 1
fi
if [ "$(latestVersion)" == "" ]; then
  echo "latest version not found, exiting..."
  exit 1
fi
TAG=$(latestVersion)
fetchAndInstall
