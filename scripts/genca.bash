#!/bin/bash

set -eu -o pipefail

# generate temporary files in deploy/genca directory
readonly SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
cd "${SCRIPT_DIR}/../deploy/genca"

# generate private key and ensure it is removed when script exits
openssl genrsa -out ./tls.key 4096
trap "rm -f tls.key" EXIT

# generate self-signed certificate and ensure key and certificate are removed when script exits
openssl req -x509 -new -nodes -key ./tls.key -out ./tls.crt -sha256 \
  -reqexts SAN \
  -extensions SAN \
  -config <(cat /etc/ssl/openssl.cnf ; printf "\n[SAN]\nsubjectAltName=DNS:telegraf-operator,DNS:telegraf-operator.telegraf-operator,DNS:telegraf-operator.telegraf-operator.svc\n\n") \
  -subj "/C=US/ST=CA/L=San Francisco/O=InfluxData/OU=IT/CA=telegraf-operator.telegraf-operator.svc" \
  -days 3650
trap "rm -f tls.key tls.crt" EXIT

# update the TLS certificate and key in dev.yml file
sed -i "s#  caBundle: .*\$#  caBundle: $(openssl base64 -e -A <./tls.crt)#" ../dev.yml
sed -i "s#  tls\.crt: .*\$#  tls.crt: $(openssl base64 -e -A <./tls.crt)#" ../dev.yml
sed -i "s#  tls\.key: .*\$#  tls.key: $(openssl base64 -e -A <./tls.key)#" ../dev.yml
