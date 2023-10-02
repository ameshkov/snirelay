# Relay

Simple relay server written in Go.

How to run locally:
```shell
LISTEN_ADDR=127.0.0.1 \
  LISTEN_PORT=8080 \
  SNI_MAPPING_CSV_PATH=sni_mapping.csv \
  VERBOSE=1 \
  ./relay

```

How to test:
```shell
# Simple connect via relay:
gocurl --connect-to="example.org:443:127.0.0.1:8080" -I https://example.org/

# Connect via relay with splitting TLS ClientHello:
gocurl --connect-to="example.org:443:127.0.0.1:8080" -I --tls-split-hello="1:50" https://example.org/

```