FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/worker ./cmd/worker

FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates qemu-utils \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --uid 10001 --create-home --shell /usr/sbin/nologin drishti \
    && install -d -o drishti -g drishti /var/lib/drishti
COPY --from=builder /out/worker /usr/local/bin/worker
USER 10001
ENV DRISHTI_WORKSPACE_ROOT=/var/lib/drishti
EXPOSE 8090
ENTRYPOINT ["/usr/local/bin/worker"]
