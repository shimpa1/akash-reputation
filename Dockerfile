# syntax=docker/dockerfile:1

# --- build the reputation server ---
FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags "-s -w" -o /out/reputation ./cmd/reputation

# --- fetch the matching provider-services binary (used by the lease poller) ---
# trixie (glibc >= 2.39) is required: provider-services 0.11.x links GLIBC_2.39.
FROM debian:trixie-slim AS pstools
ARG PROVIDER_SERVICES_VERSION=0.11.1
ARG TARGETARCH=amd64
RUN apt-get update && apt-get install -y --no-install-recommends curl unzip ca-certificates \
    && curl -fsSL -o /tmp/ps.zip \
       "https://github.com/akash-network/provider/releases/download/v${PROVIDER_SERVICES_VERSION}/provider-services_${PROVIDER_SERVICES_VERSION}_linux_${TARGETARCH}.zip" \
    && unzip -o /tmp/ps.zip -d /tmp/ps \
    && install -m 0755 /tmp/ps/provider-services /usr/local/bin/provider-services \
    && /usr/local/bin/provider-services version

# --- runtime ---
FROM debian:trixie-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --create-home --uid 10001 app
COPY --from=pstools /usr/local/bin/provider-services /usr/local/bin/provider-services
COPY --from=build /out/reputation /usr/local/bin/reputation
USER app
ENV HOME=/home/app
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/reputation"]
