FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/hypershift ./cmd/hypershift

FROM gcr.io/distroless/static-debian12
COPY --from=builder /out/hypershift /hypershift
EXPOSE 8080
ENTRYPOINT ["/hypershift"]