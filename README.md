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

## How to use it

1. Get the version for you OS/arch from the [Releases][releases] page. If you
   prefer Docker, you can find it below.
2. Create a configuration file. Read the comments in
   [./config.yaml.dist][configyaml] to learn about configuration.
3. Run `snirelay`:
    ```shell
    snirelay -c /path/to/config.yaml
    ```

   You may need to run it with `sudo` since it needs to use privileged ports.

[releases]: https://github.com/ameshkov/snirelay/releases

[configyaml]: ./config.yaml.dist

### Usage

```shell
Usage:
  snirelay [OPTIONS]

Application Options:
  -c, --config-path= Path to the config file.
  -v, --verbose      Verbose output (optional).

Help Options:
  -h, --help         Show this help message
```

## Docker

The docker image [is available][dockerregistry]. In order to use it, you need to
supply a configuration file, and you may need to also supply the TLS cert/key
if you're going to use encrypted DNS.

The image exposes a number of ports that needs to be mapped to the host machine
depending on what parts of the functionality you're using.

* Port `53`: plain DNS server, usually needs to be mapped to port `53` of the
  host machine.
* Port `853/tcp`: DNS-over-TLS server, usually needs to be mapped to port `853`
  of the host machine.
* Port `853/udp`: DNS-over-QUIC server, usually needs to be mapped to port
  `853` of the host machine.
* Port `8443/tcp`: DNS-over-HTTPS server. **Do not expose to `443` as this port
  is required by the SNI relay server**. Try a different port and don't forget
  to use it in the server address.
* Port `80/tcp`: SNI relay port for plain HTTP connections. Map it to port
  `80` of the host machine.
* Port `443/tcp`: SNI relay port for HTTPS connections. Map it to port `443` of
  the host machine.
* Port `8123/tcp`: Prometheus metrics endpoint. Map it if you use prometheus.

So imagine we have a configuration file `config.yaml` and the TLS configuration
files in the same directory in `example.crt` and `example.key`. In this case the
configuration section should look like this:

```yaml
dns:
  # ... omitted other ...
  tls-cert-path: "/app/example.crt"
  tls-key-path: "/app/example.key"
  # ... omitted other ...
```

And then run it like this:

```shell
docker run -d --name snirelay \
  -p 53:53/tcp -p 53:53/udp \
  -p 853:853/tcp -p 853:853/udp \
  -p 8443:8443/tcp \
  -p 8123:8123/tcp \
  -p 80:80/tcp -p 443:443/tcp \
  -v $(pwd)/config.yaml:/app/config.yaml \
  -v $(pwd)/example.crt:/app/example.crt \
  -v $(pwd)/example.key:/app/example.key \
  ghcr.io/ameshkov/snirelay

```

[dockerregistry]: https://github.com/ameshkov/snirelay/pkgs/container/snirelay

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

# Or you can specify the DNS server:
gocurl --dns-servers "127.0.0.1:5353" -I https://example.org/
```

[dnslookup]: https://github.com/ameshkov/dnslookup

[gocurl]: https://github.com/ameshkov/gocurl
