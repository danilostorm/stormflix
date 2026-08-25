FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/stormflix ./cmd/stormflix

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata ffmpeg && addgroup -S stormflix && adduser -S stormflix -G stormflix
WORKDIR /app
COPY --from=build /out/stormflix /usr/local/bin/stormflix
RUN mkdir -p /data && chown -R stormflix:stormflix /data
USER stormflix
EXPOSE 8090
VOLUME ["/data"]
ENTRYPOINT ["/usr/local/bin/stormflix"]
