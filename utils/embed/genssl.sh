#!/usr/bin/env bash
# generate localhost certs, valid 100 years
openssl genrsa -out localhost-CA.key 4096
openssl req -x509 -new -nodes -key localhost-CA.key -sha256 -days 1826 -out localhost-CA.crt -subj '/CN=NSPeed Root CA/C=FR/ST=Paris/L=Paris/O=NSPeed'
openssl req -x509 -newkey rsa:4096 -sha256 -days 36500 \
  -CA localhost-CA.crt -CAkey localhost-CA.key \
  -nodes -keyout localhost.key -out localhost.crt -subj "/CN=localhost" \
  -addext "subjectAltName=DNS:localhost,IP:127.0.0.1,IP:::1"
