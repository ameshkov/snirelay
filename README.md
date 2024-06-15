# SNI Relay

Simple SNI relay server written in Go.

What it does:

1. Provides a DNS server that can re-route domains to the SNI relay server.
2. Listens for incoming HTTP or HTTPS connections.
3. Parses the hostname from the HTTP request or TLS ClientHello.
4. Proxies the traffic further to that hostname.

Why would you need it? For instance, if you operate a DNS server, and you want
to relay some domains to an intermediate server (effectively, change your IP
address).

## How to build

```shell
make
```

### How to run it locally

See the [`config.yaml.dist`][configyaml] for more information on what can be
configured. In normal environment you want to change ports there.

```shell
./snirelay -c config.yaml

```

[configyaml]: ./config.yaml.dist

### How to test

Note that instructions here use [dnslookup][dnslookup] and [gocurl][gocurl].

#### DNS queries

Plain DNS:

```shell
# IPv4 will be redirected to 127.0.0.1.
dnslookup www.google.com 127.0.0.1:5353

# IPv6 will be redirected to ::.
RRTYPE=AAAA dnslookup www.google.com 127.0.0.1:5353

# HTTPS will be suppressed.
RRTYPE=HTTPS dnslookup www.google.com 127.0.0.1:5353
```

Encrypted DNS:

```shell
# DNS-over-TLS.
VERIFY=0 dnslookup www.google.com tls://127.0.0.1:8853

# DNS-over-QUIC.
VERIFY=0 dnslookup www.google.com quic://127.0.0.1:8853

# DNS-over-HTTPS.
VERIFY=0 dnslookup www.google.com https://127.0.0.1:8443/dns-query

```

#### SNI relay

```shell
# Relay for plain HTTP:
gocurl --connect-to="example.org:443:127.0.0.1:9080" -I http://example.org/

# Relay for HTTPS:
gocurl --connect-to="example.org:443:127.0.0.1:9443" -I https://example.org/

```

[dnslookup]: https://github.com/ameshkov/dnslookup

[gocurl]: https://github.com/ameshkov/gocurl

## Docker

The docker image [is available][dockerregistry]. `snirelay` listens to the
ports `8080` and `8443` inside the container, so you don't have to specify the
listen address and ports, other arguments are available.

Run `snirelay` as a background service in server mode and expose on the host's
ports `80` and `443` (tcp):

```shell
docker run -d --name snirelay \
  -p 80:8443/tcp -p 443:8443/tcp \
  ghcr.io/ameshkov/snirelay

```

[dockerregistry]: https://github.com/ameshkov/snirelay/pkgs/container/snirelay

## Usage

```text
Usage:
  snirelay [OPTIONS]

Application Options:
  -l, --listen=<IP>                                         Address the tool will be listening to (required).
  -p, --ports=<PLAIN_PORT:TLS_PORT>                         Port for accepting plain HTTP (required).
      --proxy=[protocol://username:password@]host[:port]    Proxy URL (optional).
      --sni-mappings-path=                                  Path to the file with SNI mappings (optional).
  -v, --verbose                                             Verbose output (optional).

Help Options:
  -h, --help                                                Show this help message

```

