FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/goodvibes-server ./cmd/goodvibes-server

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /out/goodvibes-server /usr/local/bin/goodvibes-server
ENV PORT=3000
ENV GOODVIBES_DB=/data/goodvibes.db
VOLUME ["/data"]
EXPOSE 3000
CMD ["goodvibes-server"]
