FROM golang:1.27.3-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY migrations ./migrations
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o /out/videohelper ./cmd/videohelper

FROM denoland/deno:bin-2.5.4 AS deno
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl ffmpeg \
    && rm -rf /var/lib/apt/lists/* \
    && curl -fL --retry 3 https://github.com/yt-dlp/yt-dlp/releases/download/2026.10.21/yt-dlp_linux -o /usr/local/bin/yt-dlp \
    && echo 'd8bd33fca9d5b23bd5b9952b3fca36db5c46331933fda9f598a3da14ce2c243f  /usr/local/bin/yt-dlp' | sha256sum -c - \
    && chmod 755 /usr/local/bin/yt-dlp
COPY --from=deno /deno /usr/local/bin/deno
COPY --from=build /out/videohelper /usr/local/bin/videohelper
ENTRYPOINT ["videohelper"]
