# ── build ───────────────────────────────────────────────────────────────
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/tickwind ./cmd/server

# ── run ─────────────────────────────────────────────────────────────────
# Not distroless: PTR parsing shells out to poppler's `pdftotext`. debian-slim +
# ca-certificates keeps HTTPS working for the static (CGO_ENABLED=0) binary,
# tzdata gives correct ET session math, and poppler-utils provides pdftotext.
FROM debian:12-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata poppler-utils && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/tickwind /tickwind
EXPOSE 8080
ENTRYPOINT ["/tickwind"]
