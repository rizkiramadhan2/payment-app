FROM golang:1.25-alpine AS builder

WORKDIR /src

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG ENV=development

RUN if [ "$ENV" != "development" ]; then \
        CGO_ENABLED=0 go build -o /bin/payment-integration-service .; \
    fi

FROM golang:1.25-alpine

WORKDIR /app

RUN apk add --no-cache git curl bash

ARG ENV=development
ENV APP_ENV=${ENV}

COPY --from=builder /src/go.mod /src/go.sum ./
RUN go mod download

COPY --from=builder /src/. .

RUN if [ "$ENV" = "development" ]; then \
        go install github.com/air-verse/air@v1.61.7; \
    fi

COPY --from=builder /bin/payment-integration-service /usr/local/bin/payment-integration-service

CMD if [ "$APP_ENV" = "development" ]; then \
        exec air -c .air.toml; \
    else \
        exec payment-integration-service; \
    fi
