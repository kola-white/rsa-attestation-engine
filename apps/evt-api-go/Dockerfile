FROM golang:1.25 AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/evt-api-go ./cmd/server

FROM gcr.io/distroless/base-debian12
WORKDIR /
COPY --from=build /out/evt-api-go /evt-api-go
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/evt-api-go"]
