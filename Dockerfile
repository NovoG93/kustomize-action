# --- build stage ---
FROM docker.io/golang:1.25 as builder
WORKDIR /src
COPY src/ ./
RUN go mod download && \
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/action .

# --- runtime stage ---
FROM debian:bookworm-slim
# hadolint ignore=DL3008
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tar git curl openssl && rm -rf /var/lib/apt/lists/*

# Install Helm 3
RUN curl -fsSL -o get_helm.sh https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 && \
    chmod 700 get_helm.sh && \
    ./get_helm.sh && \
    rm get_helm.sh

# kustomize is downloaded at runtime by the action according to env KUSTOMIZE_VERSION
COPY --from=builder /out/action /usr/local/bin/action
ENTRYPOINT ["/usr/local/bin/action"]
