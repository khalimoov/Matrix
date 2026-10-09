# Builds the Matrix server together with its web client.

FROM node:22-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.24-alpine AS server
WORKDIR /src
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /matrix ./cmd/matrix

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -H -u 1000 matrix \
    && mkdir /data && chown matrix /data
COPY --from=server /matrix /usr/local/bin/matrix
COPY --from=web /web/dist /srv/web
ENV MATRIX_WEB_DIR=/srv/web \
    MATRIX_DATA_DIR=/data \
    MATRIX_RECORDINGS_DIR=/recordings
USER matrix
VOLUME /data
EXPOSE 8080
ENTRYPOINT ["matrix"]
