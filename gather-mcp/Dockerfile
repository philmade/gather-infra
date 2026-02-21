FROM golang:1.24 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /gather-mcp .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=build /gather-mcp /usr/local/bin/gather-mcp
EXPOSE 9200
ENTRYPOINT ["gather-mcp"]
