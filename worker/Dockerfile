FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/worker ./cmd/worker

FROM gcr.io/distroless/static-debian12
COPY --from=builder /out/worker /worker
EXPOSE 8090
ENTRYPOINT ["/worker"]