FROM golang:1.26-alpine AS builder
WORKDIR /app

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -buildvcs=false -ldflags="-s -w" -o /teleport-grafana-datasource-sync .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /teleport-grafana-datasource-sync /usr/local/bin/
ENTRYPOINT ["/usr/local/bin/teleport-grafana-datasource-sync"]
