# ── build ───────────────────────────────────────────────────────────────
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/tickwind ./cmd/server

# ── run ─────────────────────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/tickwind /tickwind
EXPOSE 8080
ENTRYPOINT ["/tickwind"]
