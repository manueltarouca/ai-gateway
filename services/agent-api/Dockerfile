FROM golang:1.26.2-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /agent-api ./cmd/

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=build /agent-api /agent-api
ENTRYPOINT ["/agent-api"]
