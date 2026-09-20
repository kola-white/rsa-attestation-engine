FROM golang:1.25 AS build
WORKDIR /src

ARG BUILD_SHA=development

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/buildinfo.GitSHA=${BUILD_SHA}" \
    -o /out/evt-api-go \
    ./cmd/server

FROM gcr.io/distroless/base-debian12
WORKDIR /
COPY --from=build /out/evt-api-go /evt-api-go
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/evt-api-go"]
