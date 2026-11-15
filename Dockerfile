# ---- Build stage ----
FROM golang:alpine AS builder
RUN apk add --no-cache tzdata ca-certificates git
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/venus_server ./cmd/server

# ---- Runtime stage ----
FROM alpine:3.20
RUN apk add --no-cache tzdata ca-certificates \
    && cp /usr/share/zoneinfo/Asia/Baghdad /etc/localtime \
    && echo "Asia/Baghdad" > /etc/timezone
ENV TZ=Asia/Baghdad
WORKDIR /app
COPY --from=builder /out/venus_server /app/venus_server
COPY public /app/public
EXPOSE 8080
ENV PORT=8080
ENTRYPOINT ["/app/venus_server"]
