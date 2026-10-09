# Image for the rest-to-graphql example: the legacy REST service, the GraphQL
# service, the gate setup and the sample traffic, all in one small image.
# Built by compose.yaml in this directory; not part of mroki itself.
FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -o /out/ ./examples/rest-to-graphql/legacy ./examples/rest-to-graphql/graphql ./examples/rest-to-graphql/setup ./examples/rest-to-graphql/traffic

FROM alpine:3.21

COPY --from=builder /out/ /usr/local/bin/
COPY examples/rest-to-graphql/mapping.yaml /example/mapping.yaml

WORKDIR /example

USER 1000
