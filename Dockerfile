# ---- Build stage ----
FROM golang:1.22-alpine AS builder
RUN apk add --no-cache tzdata ca-certificates git
WORKDIR /src
# Copy full source first: `go mod tidy` needs the packages to resolve imports
# and generate go.sum for this fresh module.
COPY . .
RUN go mod tidy \
    && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/venus_server ./cmd/server

# ---- Runtime stage ----
FROM alpine:3.20
RUN apk add --no-cache tzdata ca-certificates \
    && cp /usr/share/zoneinfo/Asia/Baghdad /etc/localtime \
    && echo "Asia/Baghdad" > /etc/timezone
ENV TZ=Asia/Baghdad
WORKDIR /app
COPY --from=builder /out/venus_server /app/venus_server
# R5B: bundle the prebuilt Flutter web admin console (built into ./admin_dist).
COPY admin_dist /app/admin_dist
ENV ADMIN_DIST_DIR=/app/admin_dist
EXPOSE 8080
ENV PORT=8080
ENTRYPOINT ["/app/venus_server"]
