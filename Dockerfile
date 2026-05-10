FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/milod ./cmd/milod

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /out/milod /usr/local/bin/milod
ENTRYPOINT ["/usr/local/bin/milod"]
