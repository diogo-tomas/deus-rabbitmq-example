# syntax=docker/dockerfile:1

FROM golang:1.24-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/worker ./cmd/worker
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate

FROM alpine:3.21

RUN addgroup -S app && adduser -S -G app app \
    && apk add --no-cache ca-certificates wget

WORKDIR /app

COPY --from=build /out/api /app/api
COPY --from=build /out/worker /app/worker
COPY --from=build /out/migrate /app/migrate

USER app

EXPOSE 8080

CMD ["/app/api"]
