# Builds any service: docker build --build-arg SERVICE=ledger-service .
FROM golang:1.26.11-alpine AS build
ARG SERVICE
WORKDIR /src
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    cd services/${SERVICE} && GOWORK=off CGO_ENABLED=0 \
    go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /out/server /server
USER 65532:65532
ENTRYPOINT ["/server"]
