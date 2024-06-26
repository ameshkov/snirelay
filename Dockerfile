# Step 1: Use the official Golang image as the build environment.
# This image includes all the tools needed to compile Go applications.
FROM golang:1.22 as builder

# Version will be passed as a part of the build.
ARG VERSION=dev

# Set the Current Working Directory inside the container.
WORKDIR /app

# Copy the local package files to the container's workspace.
COPY . .

# Build the Go app for a Linux system. The -o flag specifies the output binary
# name.
RUN VERSION=${VERSION} make build

# Step 2: Use a builder image to get the latest certificates.
FROM alpine:latest as certs

# Download the latest CA certificates
RUN apk --update add ca-certificates

# Step 3: Use a Docker multi-stage build to create a lean production image.
# Start from a scratch (empty) image to keep the image size small.
FROM scratch

# Copy the binary from the builder stage to the production image.
COPY --from=builder /app/snirelay /

# Copy the CA certificates from the certs image.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Exposing ports.

# Plain DNS.
EXPOSE 53/udp
EXPOSE 53/tcp

# DNS-over-TLS.
EXPOSE 853/tcp

# DNS-over-QUIC.
EXPOSE 853/udp

# DNS-over-HTTPS.
EXPOSE 8443/tcp

# SNI relay for plain HTTP.
EXPOSE 80/tcp

# SNI relay for HTTPS.
EXPOSE 443/tcp

# Prometheus metrics endpoint.
EXPOSE 8123/tcp

ENTRYPOINT ["/snirelay", "-c", "/app/config.yaml"]