FROM golang:1.27-alpine AS build
RUN apk add --no-cache git
RUN CGO_ENABLED=0 go install github.com/minio/minio@v0.0.0-20251015172955-9e49d5e7a648

FROM alpine:3.24
RUN apk add --no-cache ca-certificates curl
COPY --from=build /go/bin/minio /usr/local/bin/minio
EXPOSE 9000 9001
ENTRYPOINT ["minio"]
