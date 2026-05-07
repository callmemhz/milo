FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/milo-apps-kit-server ./cmd/milo-apps-kit-server

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /out/milo-apps-kit-server /usr/local/bin/milo-apps-kit-server
ENTRYPOINT ["/usr/local/bin/milo-apps-kit-server"]
