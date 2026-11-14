# admin_dist

Build output of the Flutter web admin console is placed here:

    flutter build web --release -t lib/main_admin.dart --base-href "/" -o admin_dist

The Go server serves this directory at `/` and `/console` (see `cmd/server/console.go`).
This placeholder keeps `COPY admin_dist /app/admin_dist` in the Dockerfile working
before the first web build; it is replaced by the real build output.
